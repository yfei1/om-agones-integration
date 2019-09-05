package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"open-match.dev/open-match/pkg/pb"

	agonesv1 "agones.dev/agones/pkg/apis/agones/v1"
	allocationv1 "agones.dev/agones/pkg/apis/allocation/v1"
	autoscalerv1 "agones.dev/agones/pkg/apis/autoscaling/v1"
	"agones.dev/agones/pkg/client/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
)

func doSomething() error {
	bc, closer := getOMBackendClient(cfg)
	defer closer()

	fmReq := &pb.FetchMatchesRequest{
		Config: &pb.FunctionConfig{
			Host: "om-function",
			Port: 50502,
			Type: pb.FunctionConfig_GRPC,
		},
		Profiles: []*pb.MatchProfile{
			{
				Name: "some-profile",
				Pools: []*pb.Pool{
					{
						Name: "some-name",
						Filters: []*pb.Filter{
							{
								Attribute: "mmr.rating",
								Min:       0,
								Max:       20,
							},
						},
					},
				},
			},
		},
	}

	stream, err := bc.FetchMatches(context.Background(), fmReq)
	if err != nil {
		fmt.Printf("Director: fail to get response stream from backend.FetchMatches call, desc: %s\n", err.Error())
		return err
	}

	matches := []*pb.Match{}
	playerStr := ""
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Director: fail to read response from backend.FetchMatches stream, desc: %s\n", err.Error())
			return err
		}
		matches = append(matches, resp.GetMatch())
		for _, match := range matches {
			for _, ticket := range match.GetTickets() {
				playerStr += " " + ticket.GetProperties().Fields["name"].String()
			}
		}
	}

	fmt.Printf("Director: got %d matches from OM backend for players %s\n", len(matches), playerStr)

	agonesClient, err := versioned.NewForConfig(cfg)
	if err != nil {
		fmt.Println("Could not create the agones api clientset")
		return err
	}

	// Ask Agones for a game server and allocate tickets to the server
	for _, match := range matches {
		gsa, err := agonesClient.AllocationV1().GameServerAllocations("default").Create(&allocationv1.GameServerAllocation{
			Spec: allocationv1.GameServerAllocationSpec{
				Required: metav1.LabelSelector{
					MatchLabels: map[string]string{agonesv1.FleetNameLabel: "simple-udp"},
				},
			},
		})
		if err != nil {
			fmt.Printf("Director: failed to create game server allocation, desc: %s\n", err.Error())
			return err
		}

		if gsa.Status.State == allocationv1.GameServerAllocationAllocated {
			fmt.Printf(
				"Director: created an allocation. State: %s, GameServerName: %s, Port: %d, Address: %s, NodeName: %s\n",
				gsa.Status.State,
				gsa.Status.GameServerName,
				gsa.Status.Ports[0].Port,
				gsa.Status.Address,
				gsa.Status.NodeName,
			)

			_, err = bc.AssignTickets(context.Background(), &pb.AssignTicketsRequest{
				TicketIds: getTicketIds(match.GetTickets()),
				Assignment: &pb.Assignment{
					Connection: fmt.Sprintf("%s:%d", gsa.Status.Address, gsa.Status.Ports[0].Port),
				},
			})
			if err != nil {
				fmt.Printf("Director: failed to assign tickets to game server, desc: %s\n", err.Error())
				fmt.Printf("Director: attempt to cleanup the staled game server %s\n", gsa.Status.GameServerName)
				if err = agonesClient.AgonesV1().GameServers("default").Delete(gsa.Status.GameServerName, &metav1.DeleteOptions{}); err != nil {
					fmt.Printf("Director: failed to garbage collect the assigned game server %s, desc: %s\n", gsa.Status.GameServerName, err.Error())
				}
				return err
			}
		} else {
			fmt.Printf("Director: failed to allocate game server.\n")
		}
	}

	fmt.Printf("Director: assigned %d matches to agones in this cycle\n", len(matches))
	time.Sleep(time.Second * 5)
	return nil
}

func getTicketIds(tickets []*pb.Ticket) []string {
	tids := []string{}
	for _, t := range tickets {
		tids = append(tids, t.GetId())
	}
	return tids
}

func getOMBackendClient(cfg *rest.Config) (pb.BackendClient, func() error) {
	conn := getGRPCConnFromSvcName(cfg, "om-backend")
	return pb.NewBackendClient(conn), conn.Close
}

func initialize() {
	// Access to the Agones resources through the Agones Clientset
	// Note that we use the same config as we used for the Kubernetes Clientset
	agonesClient, err := versioned.NewForConfig(getKubeConfig())
	if err != nil {
		panic("Could not create the agones api clientset")
	}

	// Create a Fleet of warm GameServers under the default namespace
	_, err = agonesClient.AgonesV1().Fleets("default").Create(&agonesv1.Fleet{
		ObjectMeta: metav1.ObjectMeta{Name: "simple-udp"},
		Spec: agonesv1.FleetSpec{
			Replicas: 2,
			Template: agonesv1.GameServerTemplateSpec{
				Spec: agonesv1.GameServerSpec{
					Ports: []agonesv1.GameServerPort{
						{
							Name:          "default",
							ContainerPort: 7654,
						},
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "simple-udp",
									Image: "gcr.io/agones-images/udp-server:0.14",
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	// Create a FleetAutoscaler to manage fleet size automatically - creates new game server with buffer size 2
	_, err = agonesClient.AutoscalingV1().FleetAutoscalers("default").Create(&autoscalerv1.FleetAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "simple-udp-autoscaler"},
		Spec: autoscalerv1.FleetAutoscalerSpec{
			FleetName: "simple-udp",
			Policy: autoscalerv1.FleetAutoscalerPolicy{
				Type: "Buffer",
				Buffer: &autoscalerv1.BufferPolicy{
					BufferSize:  intstr.FromInt(2),
					MinReplicas: 0,
					MaxReplicas: 10,
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}

}

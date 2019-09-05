package main

import (
	"context"
	"fmt"
	"time"

	"agones.dev/agones/pkg/client/clientset/versioned"
	"open-match.dev/open-match/pkg/pb"
	"open-match.dev/open-match/pkg/structs"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
)

func enterQueue(idc <-chan string) <-chan string {
	// Generate a fake ticket
	fc, closer := getOMFrontendClient(cfg)

	pid := <-idc
	// Create tickets in Open Match
	resp, err := fc.CreateTicket(context.Background(), &pb.CreateTicketRequest{
		Ticket: &pb.Ticket{
			Properties: structs.Struct{
				"name":       structs.String(string(pid)),
				"mmr.rating": structs.Number(float64(10)),
			}.S(),
		},
	})
	if err != nil {
		panic(err)
	}

	gsc := make(chan string, 1)
	tid := resp.GetTicket().GetId()

	go func() {
		defer func() {
			closer()
			close(gsc)
		}()
		for {
			select {
			case <-idc:
				// The channel is closed, indicates the player exit the lobby
				fmt.Printf("Game Frontend: %s exit the lobby\n", pid)
				// Recycle the game server if the player quit after getting assigned
				ticket, err := fc.GetTicket(context.Background(), &pb.GetTicketRequest{TicketId: tid})
				if err != nil {
					fmt.Printf("Game Frontend: failed to get ticket %s for verification", tid)
					return
				}
				if ticket.GetAssignment().GetConnection() != "" {
					agonesClient, err := versioned.NewForConfig(cfg)
					if err != nil {
						fmt.Println("Game Frontend: failed to get agonesClient from kubeConfig")
						return
					}
					staledGameServerName := ticket.GetAssignment().GetProperties().GetFields()["gsName"].GetStringValue()
					if err = agonesClient.AgonesV1().GameServers("default").Delete(staledGameServerName, &metav1.DeleteOptions{}); err != nil {
						fmt.Printf("Game Frontend: failed to recycle the staled game server %s\n", staledGameServerName)
						return
					}
					fmt.Printf("Game Frontend: recycled a staled game server %s since player canceled the match\n", staledGameServerName)
					return
				}
				// Stop finding a match for this player if the player cancels the lookup or gets an assignment
				fc.DeleteTicket(context.Background(), &pb.DeleteTicketRequest{TicketId: tid})
				return
			default:
				// Player is still waiting for a match
				t, err := fc.GetTicket(context.Background(), &pb.GetTicketRequest{TicketId: tid})
				if err != nil {
					panic(err)
				}
				tconn := t.GetAssignment().GetConnection()
				if tconn == "" {
					time.Sleep(1 * time.Second)
				} else {
					gsc <- tconn
					fmt.Printf("Game Frontend: game server found for %s\n", pid)
					// Stop finding a match for this player if the player cancels the lookup or gets an assignment
					fc.DeleteTicket(context.Background(), &pb.DeleteTicketRequest{TicketId: tid})
					return
				}
			}
		}
	}()
	return gsc
}

func getOMFrontendClient(cfg *rest.Config) (pb.FrontendClient, func() error) {
	conn := getGRPCConnFromSvcName(cfg, "om-frontend")
	return pb.NewFrontendClient(conn), conn.Close
}

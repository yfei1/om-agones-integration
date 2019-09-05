# Agones sample integration with Open Match
The goal of this integration is to have a local/out-of-cluster game backend & frontend talking to a remote cluster with Agones and Open Match installed in different namespaces using the simple-udp game.

## Step 1 - Create Cluster:
```bash
gcloud container clusters create integration --cluster-version=1.12 \
  --tags=integration --scopes=gke-default --num-nodes=4 --machine-type=n1-standard-8
```

## Step 2 - Create a Node Pool for Agones:
```bash
gcloud container node-pools create agones-system \
  --cluster=integration \
  --node-taints agones.dev/agones-system=true:NoExecute \
  --node-labels agones.dev/agones-system=true \
  --num-nodes=1
```

## Step 3 - Auth the cluster:
```bash
gcloud config set container/cluster integration
gcloud container clusters get-credentials integration
```

## Step 4 - Set up firewall policy (Open a port for game server connection)
Open UDP port 7000-8000 for Agones' simple-udp game 
```bash
gcloud compute firewall-rules create gke-integration-fw-rules \
  --allow udp:7000-8000 \
  --target-tags integration \
  --description "Firewall to allow game server udp traffic"

```

## Step 5 - Install Agones under the `agones-system` namespace
```bash
kubectl create namespace agones-system
kubectl apply -f https://raw.githubusercontent.com/googleforgames/agones/release-0.12.0/install/yaml/install.yaml
```

## Step 6 - Install Open Match under the `open-match` namespace
```bash
kubectl create namespace open-match
kubectl apply -f https://raw.githubusercontent.com/yfei1/om-agones-integration/master/output_01-open-match-core.yaml -n open-match
```

## Step 7 - Install customized components for Open Match using the example MMF and Evaluator
We don't have a v0.7 release yet so I'm using the example MMF and Evaluator image based on HEAD of the Open Match repo in my personal registry
```bash
kubectl apply -f https://raw.githubusercontent.com/yfei1/om-agones-integration/master/output_open-match-example-customized-components.yaml
```

## Step 8 - Run the example
```bash
GO111MODULE=on go run main.go
```

package main

import (
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var cfg = getKubeConfig()

func main() {
	initializeAgonesSettings()
	simulatePlayer()
	for {
		if err := directGamePlay(); err != nil {
			panic(err)
			break
		}
	}
}

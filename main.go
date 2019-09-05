package main

import (
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var cfg = getKubeConfig()

func main() {
	// initialize()
	simulate()
	for {
		if err := doSomething(); err != nil {
			panic(err)
			break
		}
	}
}

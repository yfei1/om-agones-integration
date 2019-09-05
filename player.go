package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"net"
	"time"
)

func simulatePlayer() {
	for i := 0; i < 2; i++ {
		go func(i int) {
			for {
				play(i)
				fmt.Printf("Player %d: need a short break\n", i)
				time.Sleep(10 * time.Second)
			}
		}(i)
	}
}

func play(i int) {
	pc := make(chan string, 1)
	defer close(pc)
	pc <- fmt.Sprintf("player-%d", i)

	gsc := enterQueue(pc)

	for {
		select {
		case gsAddr := <-gsc:
			// Game Frontend returns a connection for the player
			raddr, err := net.ResolveUDPAddr("udp", gsAddr)
			if err != nil {
				fmt.Println(err.Error())
				return
			}

			// Player connects to the game server
			conn, err := net.DialUDP("udp", nil, raddr)
			if err != nil {
				fmt.Printf("Player %d: fail to DialUDP", i)
				return
			}
			defer conn.Close()
			if err != nil {
				fmt.Printf("Player %d: fail to establish connection to game server %s, desc: %s\n", i, gsAddr, err.Error())
				return
			}
			// Player sends something to the game server
			if _, err = fmt.Fprintf(conn, "Hello Game Server"); err != nil {
				fmt.Printf("Player %d: fail to say hello to game server %s, desc: %s\n", i, gsAddr, err.Error())
				return
			}

			// Player reads response from the game server
			p := make([]byte, 1024)
			if _, err = bufio.NewReader(conn).Read(p); err != nil {
				fmt.Printf("Player %d: fail to read response from game server: %s, desc: %s\n", i, gsAddr, err.Error())
				return
			}

			fmt.Printf("Player %d: game server %s ack with: %s\n", i, gsAddr, string(p))

			// Who needs sleep? Sleep is for the weak
			time.Sleep(time.Duration(rand.New(rand.NewSource(time.Now().UnixNano()+int64(i*10))).Intn(5)) * time.Second)

			deadline := time.Now().Add(time.Second)
			err = conn.SetDeadline(deadline)
			if err != nil {
				fmt.Printf("Player %d: failed to set ddl for connection", i)
				return
			}
			p = make([]byte, 1024)
			// Player attempts to exit the game
			if _, err = fmt.Fprint(conn, "EXIT"); err != nil {
				fmt.Printf("Player %d: fail to say goodbye to game server %s, desc: %s\n", i, gsAddr, err.Error())
				return
			}

			if _, _, err = conn.ReadFrom(p); err != nil {
				fmt.Printf("Player %d: fail to read response from game server: %s, desc: %s\n", i, gsAddr, err.Error())
				return
			}

			fmt.Printf("Player %d: left the game, game server respond with %s\n", i, string(p))
			return
		default:
			// Simulate an angry player that is likely to cancel the lookup
			rd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i*10)))
			if rd.Intn(10) == 0 {
				fmt.Printf("Player %d: got angry and cancel the lookup\n", i)
				return
			} else {
				time.Sleep(1 * time.Second)
			}
		}
	}
}

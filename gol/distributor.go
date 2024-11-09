package gol

import (
	"fmt"
	"net/rpc"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	KeyPresses <-chan rune
}

type Request struct {
	Params Params
	World  [][]byte
	Turns  int
}

type Response struct {
	LastWorld  [][]byte
	AliveCells []util.Cell
	Turns      int
}

func saveWorld(c distributorChannels, world [][]byte, p Params) {
	c.ioCommand <- ioOutput
	filename := fmt.Sprintf("%dx%dx%d", p.ImageWidth, p.ImageHeight, p.Turns)
	c.ioFilename <- filename

	for y := 0; y < len(world); y++ {
		for x := 0; x < len(world[0]); x++ {
			c.ioOutput <- world[y][x]
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	turn := 0
	c.ioCommand <- ioInput
	filename := fmt.Sprintf("%dx%d", p.ImageWidth, p.ImageHeight)
	c.ioFilename <- filename
	serverAddr := "127.0.0.1:8030"

	client, _ := rpc.Dial("tcp", serverAddr)
	defer client.Close()

	// Load initial world state from input
	World := make([][]byte, p.ImageHeight)
	for i := range World {
		World[i] = make([]byte, p.ImageWidth)
	}
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			World[y][x] = <-c.ioInput
		}
	}

	request := Request{
		Params: p,
		World:  World,
	}

	done := make(chan struct{})
	paused := false

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var mu sync.Mutex

	// Goroutine to handle keyboard events and send events
	go func() {
		for {
			select {
			case key := <-c.KeyPresses:
				var response Response
				client.Call("GameOfLife.GetInfo", nil, &response)
				World = response.LastWorld
				switch key {
				case 'p':
					mu.Lock()
					if !paused {
						c.events <- StateChange{turn, Paused}
						client.Call("GameOfLife.Pause", nil, nil)
						fmt.Println("Engine paused!")
						paused = true
					} else {
						client.Call("GameOfLife.Unpause", nil, nil)
						paused = false
						c.events <- StateChange{turn, Executing}
					}
					mu.Unlock()
				case 's':
					mu.Lock()
					c.events <- StateChange{turn, Executing}
					saveWorld(c, World, p)
					mu.Unlock()
				case 'q':
					mu.Unlock()
					client.Call("GameOfLife.Quit", nil, nil)
					c.events <- StateChange{turn, Quitting}
					saveWorld(c, World, p)
					mu.Unlock()
				case 'k':
					client.Call("GameOfLife.Kill", nil, nil)
					c.events <- StateChange{turn, Quitting}
					saveWorld(c, World, p)
					close(c.events)
				}
			case <-ticker.C:
				mu.Lock()
				var response Response
				client.Call("GameOfLife.SendAlive", request, &response)
				foundAlive := len(response.AliveCells)
				c.events <- AliveCellsCount{response.Turns, foundAlive}
				mu.Unlock()
			case <-done:
				return
			}
		}
	}()

	var alive []util.Cell
	for turn < p.Turns {
		var response Response
		client.Call("GameOfLife.ProcessTurns", request, &response)

		// Update the world state and turn
		mu.Lock()
		turn++
		request.World = response.LastWorld
		World = response.LastWorld
		mu.Unlock()
		c.events <- TurnComplete{CompletedTurns: turn}

		alive = response.AliveCells
	}

	// Clean up and finalize
	close(done)
	c.events <- FinalTurnComplete{CompletedTurns: turn, Alive: alive}

	// Output the final world state
	saveWorld(c, World, p)
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	outputFilename := fmt.Sprintf("%dx%d", p.ImageWidth, p.ImageHeight)
	c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: outputFilename}

	c.events <- StateChange{turn, Quitting}
	close(c.events)
}

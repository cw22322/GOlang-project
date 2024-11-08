package gol

import (
	"fmt"
	"net/rpc"
	"strconv"
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
	quitting := false

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var mu sync.Mutex

	// Goroutine to handle keyboard events and send events
	go func() {
		for {
			select {
			case key := <-c.KeyPresses:
				switch key {
				case 'p':
					mu.Lock()
					paused = !paused
					if paused {
						c.events <- StateChange{turn, Paused}
					} else {
						c.events <- StateChange{turn, Executing}
					}
					mu.Unlock()
				case 's':
					mu.Lock()
					var response Response
					client.Call("GameOfLife.Save", request, &response)
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle

					c.ioCommand <- ioOutput
					filename = filename + "x" + strconv.Itoa(p.Turns)
					c.ioFilename <- filename

					world := response.LastWorld
					for y := 0; y < len(world); y++ {
						for x := 0; x < len(world[y]); x++ {
							c.ioOutput <- world[y][x]
						}
					}

					c.ioCommand <- ioCheckIdle
					<-c.ioIdle
					mu.Unlock()
					c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: filename}
				case 'q':
					mu.Lock()
					quitting = true
					c.events <- StateChange{turn, Quitting}
					mu.Unlock()
					return
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
		mu.Lock()
		if quitting {
			mu.Unlock()
			break
		}
		if paused {
			mu.Unlock()
			time.Sleep(100 * time.Millisecond)
			continue
		}
		mu.Unlock()

		// Process one turn
		singleTurnParams := p
		singleTurnParams.Turns = 1
		request.Params = singleTurnParams

		var response Response
		client.Call("GameOfLife.ProcessTurns", request, &response)

		// Update the world state and turn
		mu.Lock()
		turn++
		request.World = response.LastWorld
		mu.Unlock()

		alive = response.AliveCells
	}

	// Clean up and finalize
	close(done)
	c.events <- FinalTurnComplete{CompletedTurns: turn, Alive: alive}

	// Output the final world state
	c.ioCommand <- ioOutput
	filename = filename + "x" + strconv.Itoa(p.Turns)
	c.ioFilename <- filename
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- request.World[y][x]
		}
	}
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	close(c.events)
}

package gol

import (
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
	var turn int
	c.ioCommand <- ioInput
	filename := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	c.ioFilename <- filename
	serverAddr := "127.0.0.1:8030"

	client, _ := rpc.Dial("tcp", serverAddr)
	defer client.Close()

	// Load initial world state from input
	World := make([][]byte, p.ImageHeight)
	for i := range World {
		World[i] = make([]byte, p.ImageWidth)
	}
	for i := range World {
		for j := 0; j < p.ImageWidth; j++ {
			World[i][j] = <-c.ioInput
		}
	}

	request := Request{
		Params: p,
		World:  World,
		Turns:  p.Turns,
	}

	done := make(chan struct{})
	paused := false
	quitting := false
	stateChan := make(chan State, 1)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var mu sync.Mutex
	go func() {
		var response Response
		for {
			select {
			case key := <-c.KeyPresses:
				switch key {
				case 'p':
					mu.Lock()
					paused = true
					if paused {
						c.events <- StateChange{turn, Paused}
					} else {
						stateChan <- Executing
						c.events <- StateChange{turn, Executing}
					}
					mu.Unlock()
				case 's':
					mu.Lock()
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle

					c.ioCommand <- ioOutput
					filename = filename + "x" + strconv.Itoa(p.Turns)
					c.ioFilename <- filename

					client.Call("GameOfLife.Save", request, &response)
					worldSend := response.LastWorld
					for y := 0; y < len(response.LastWorld); y++ {
						for x := 0; x < len(worldSend[0]); x++ {
							c.ioOutput <- World[y][x]
						}
					}
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle
					mu.Unlock()
					c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: filename}
				case 'q':
					mu.Lock()
					quitting = true
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle
					stateChan <- Quitting
					mu.Unlock()
				}
			case <-ticker.C:
				mu.Lock()
				client.Call("GameOfLife.SendAlive", request, &response)
				foundalive := len(response.AliveCells)
				c.events <- AliveCellsCount{response.Turns, foundalive}
				mu.Unlock()
			case <-done:
				return
			}
		}
	}()

	var response Response
	client.Call("GameOfLife.ProcessTurns", request, &response)
	close(done)
	World = response.LastWorld
	alive := response.AliveCells
	turn = response.Turns

	ticker.Stop()

	c.events <- FinalTurnComplete{CompletedTurns: response.Turns, Alive: alive}

	c.ioCommand <- ioOutput
	filename = filename + "x" + strconv.Itoa(p.Turns)
	c.ioFilename <- filename
	for y := 0; y < len(World); y++ {
		for x := 0; x < len(World[0]); x++ {
			c.ioOutput <- World[y][x]
		}
	}
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{p.Turns, Quitting}
	close(c.events)
}

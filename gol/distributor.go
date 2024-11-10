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
	keyPresses <-chan rune
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

func distributor(p Params, c distributorChannels) {
	c.ioCommand <- ioInput
	filename := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	c.ioFilename <- filename
	serverAddr := "127.0.0.1:8030"

	client, _ := rpc.Dial("tcp", serverAddr)
	defer client.Close()

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
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var mu sync.Mutex
	go func() {
		var response Response
		paused := false
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case key := <-c.keyPresses:
				switch key {
				case 'p':
					if paused == false {
						c.events <- StateChange{response.Turns, Paused}
						client.Call("GameOfLife.Pause", nil, nil)
						paused = true
					} else {
						paused = false
						c.events <- StateChange{response.Turns, Executing}
						client.Call("GameOfLife.Unpause", nil, nil)
					}

				case 's':
					mu.Lock()
					client.Call("GameOfLife.Save", request, &response)
					c.ioCommand <- ioOutput
					filename = filename + "x" + strconv.Itoa(p.Turns)
					c.ioFilename <- filename
					worldSend := response.LastWorld
					for y := 0; y < len(response.LastWorld); y++ {
						for x := 0; x < len(worldSend[0]); x++ {
							c.ioOutput <- World[y][x]
						}
					}
					mu.Unlock()
				case 'q':
					mu.Lock()
					c.events <- StateChange{response.Turns, Quitting}
					c.ioCommand <- ioOutput
					filename = filename + "x" + strconv.Itoa(p.Turns)
					c.ioFilename <- filename
					worldSend := response.LastWorld
					for y := 0; y < len(response.LastWorld); y++ {
						for x := 0; x < len(worldSend[0]); x++ {
							c.ioOutput <- World[y][x]
						}
					}
					close(c.events)
					mu.Unlock()
					return
				case 'k':
					mu.Lock()
					client.Call("GameOfLife.Kill", request, &response)
					c.events <- StateChange{response.Turns, Quitting}
					c.ioCommand <- ioOutput
					filename = filename + "x" + strconv.Itoa(p.Turns)
					c.ioFilename <- filename
					worldSend := response.LastWorld
					for y := 0; y < len(response.LastWorld); y++ {
						for x := 0; x < len(worldSend[0]); x++ {
							c.ioOutput <- World[y][x]
						}
					}
					close(c.events)
					mu.Unlock()
					return
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

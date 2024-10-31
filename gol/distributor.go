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

func calculateAliveCells(p Params, world [][]byte) []util.Cell {
	var alivecells []util.Cell
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] != 0 {
				alivecells = append(alivecells, util.Cell{X: x, Y: y})
			}
		}
	}
	return alivecells
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
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

	var mu sync.Mutex
	go func() {
		var response Response
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:

				mu.Lock()
				client.Call("GameOfLife.SendAlive", request, &response)
				foundalive := len(response.AliveCells)
				c.events <- AliveCellsCount{response.Turns, foundalive}
				mu.Unlock()
			default:
				if response.Turns == p.Turns {
					return
				}

			}
		}
	}()

	var response Response
	client.Call("GameOfLife.ProcessTurns", request, &response)
	World = response.LastWorld
	alive := calculateAliveCells(p, World)
	c.events <- FinalTurnComplete{CompletedTurns: p.Turns, Alive: alive}

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

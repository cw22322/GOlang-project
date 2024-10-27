package gol

import (
	"fmt"
	"strconv"
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

func calculateNextState(p Params, world [][]byte) [][]byte {
	newWorld := make([][]byte, p.ImageHeight)
	for i := range newWorld {
		newWorld[i] = make([]byte, p.ImageWidth)
	}

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			alive := 0

			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dy == 0 && dx == 0 {
						continue
					}

					nx, ny := dx+x, dy+y

					if nx < 0 {
						nx = p.ImageWidth - 1
					}

					if ny < 0 {
						ny = p.ImageHeight - 1
					}

					if nx >= p.ImageWidth {
						nx = 0
					}

					if ny >= p.ImageHeight {
						ny = 0
					}

					if world[ny][nx] == 255 {
						alive++
					}

				}

			}
			if world[y][x] == 255 {
				if alive < 2 {
					newWorld[y][x] = 0
				} else if alive == 2 || alive == 3 {
					newWorld[y][x] = 255
				} else {
					newWorld[y][x] = 0
				}
			} else {
				if alive == 3 {
					newWorld[y][x] = 255

				} else {
					newWorld[y][x] = 0
				}
			}
		}
	}

	return newWorld
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	//RPC Dial
	/*client, err := rpc.Dial("tcp", "127.0.0.1:8030")
	if err != nil {
		log.Fatal("Cannot connect to server:", err)
	}*/

	var filename = strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + ".pmg"
	fmt.Println(filename)
	// rows := p.ImageHeight / p.Threads

	World := make([][]uint8, p.ImageHeight)
	for i := range World {
		World[i] = make([]uint8, p.ImageWidth)
		for j := 0; j < p.ImageWidth; j++ {
		}
	}

	for i := range World {
		for j := 0; j < p.ImageWidth; j++ {
			World[i][j] = <-c.ioInput
		}
	}

	c.ioCommand <- ioInput
	c.ioFilename <- filename

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {

			value, _ := <-c.ioInput // Receive a value from the channel

			World[y][x] = value
		}
	}

	// TODO: Create a 2D slice to store the world.

	turn := 0
	c.events <- StateChange{turn, Executing}

	// TODO: Execute all turns of the Game of Life.

	for turn = 0; turn < p.Turns; turn++ {
		World = calculateNextState(p, World)
	}
	alive := calculateAliveCells(p, World)

	// TODO: Report the final state using FinalTurnCompleteEvent.

	Finalturn := FinalTurnComplete{
		CompletedTurns: turn,
		Alive:          alive,
	}

	c.events <- Finalturn

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	//false <- c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

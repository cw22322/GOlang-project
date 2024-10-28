package gol

import (
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

func worker(startY, endY int, p Params, out chan<- [][]byte) {
	worldSlice := make([][]uint8, endY-startY)
	for i := range worldSlice {
		worldSlice[i] = make([]uint8, p.ImageWidth)
	}
	out <- worldSlice
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	c.ioCommand <- ioInput
	filename := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	c.ioFilename <- filename

	World := make([][]uint8, p.ImageHeight)
	for i := range World {
		World[i] = make([]uint8, p.ImageWidth)
	}

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			World[y][x] = <-c.ioInput
		}
	}

	workerHeight := p.ImageHeight / p.Threads
	out := make([]chan [][]uint8, p.Threads)
	for i := range out {
		out[i] = make(chan [][]uint8)
	}

	turn := 0
	c.events <- StateChange{turn, Executing}

	// TODO: Execute all turns of the Game of Life.

	newWorld := make([][]uint8, p.ImageHeight)

	for turn = 0; turn < p.Turns; turn++ {

		for i := 0; i < p.Threads; i++ {
			go worker(i*workerHeight, (i+1)*workerHeight, World, p, out[i])
		}

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

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

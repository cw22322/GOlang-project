package gol

import (
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

func calculateAliveCells(p Params, world [][]byte) []util.Cell {
	var alivecells []util.Cell
	for y := 0; y < len(world); y++ {
		for x := 0; x < len(world[0]); x++ {
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

	for y := 0; y < len(world); y++ {
		for x := 0; x < len(world[0]); x++ {
			alive := 0

			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dy == 0 && dx == 0 {
						continue
					}

					nx, ny := dx+x, dy+y

					if nx < 0 {
						nx = len(world[0]) - 1
					}

					if ny < 0 {
						ny = len(world) - 1
					}

					if nx >= len(world[0]) {
						nx = 0
					}

					if ny >= len(world) {
						ny = 0
					}

					if world[ny][nx] == 255 {
						alive++
					}

				}

			}
			if world[y][x] == 255 {
				if alive < 2 || alive > 3 {
					newWorld[y][x] = 0
				} else {
					newWorld[y][x] = 255
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

func worker(startY, endY int, p Params, world [][]byte, out chan<- [][]byte) {

	worldSlice := world[startY:endY]

	newWorld := calculateNextState(p, worldSlice)

	out <- newWorld
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	turn := 0
	c.ioCommand <- ioInput
	filename := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	c.ioFilename <- filename

	World := make([][]byte, p.ImageHeight)
	for i := range World {
		World[i] = make([]byte, p.ImageWidth)
	}

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			World[y][x] = <-c.ioInput
		}
	}

	workerHeight := p.ImageHeight / p.Threads
	out := make([]chan [][]byte, p.Threads)
	for i := range out {
		out[i] = make(chan [][]byte)
	}
	var mu sync.Mutex

	c.events <- StateChange{turn, Executing}

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				c.events <- AliveCellsCount{turn, len(calculateAliveCells(p, World))}
				mu.Unlock()
			default:
				if turn == p.Turns {
					return
				}

			}
		}
	}()

	go func() {
		filename = filename + "x" + strconv.Itoa(turn)
		for {
			select {
			case key := <-c.keyPresses:
				switch key {
				case 's':

					for y := 0; y < len(World); y++ {
						for x := 0; x < len(World[0]); x++ {
							c.ioOutput <- World[y][x]
						}
					}
					c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: filename}
				case 'q':
					alive := calculateAliveCells(p, World)
					for y := 0; y < len(World); y++ {
						for x := 0; x < len(World[0]); x++ {
							c.ioOutput <- World[y][x]
						}
					}
					c.events <- FinalTurnComplete{CompletedTurns: turn, Alive: alive}
					c.events <- StateChange{turn, Quitting}
					close(c.events)
					return
				case 'p':
					c.events <- StateChange{turn, Paused}
					paused := true
					if paused {
						command := <-c.keyPresses
						switch command {
						case 'p':
							paused = false
							c.events <- StateChange{turn, Executing}
						case 's':
							c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: filename}
							for y := 0; y < len(World); y++ {
								for x := 0; x < len(World[0]); x++ {
									c.ioOutput <- World[y][x]
								}
							}
						case 'q':
							alive := calculateAliveCells(p, World)
							c.events <- FinalTurnComplete{CompletedTurns: turn, Alive: alive}
							c.events <- StateChange{turn, Quitting}
							for y := 0; y < len(World); y++ {
								for x := 0; x < len(World[0]); x++ {
									c.ioOutput <- World[y][x]
								}
							}
							close(c.events)
							return
						}
					}
				}
			}
		}
	}()

	// TODO: Execute all turns of the Game of Life.

	for turn = 0; turn < p.Turns; turn++ {
		// Start worker goroutines
		for i := 0; i < p.Threads; i++ {
			go worker(i*workerHeight, (i+1)*workerHeight, p, World, out[i])
		}

		mu.Lock()
		World = calculateNextState(p, World)
		mu.Unlock()

	}
	alive := calculateAliveCells(p, World)

	// TODO: Report the final state using FinalTurnCompleteEvent.

	Finalturn := FinalTurnComplete{
		CompletedTurns: turn,
		Alive:          alive,
	}

	c.events <- Finalturn

	// Make sure that the Io has finished any output before exiting.

	c.ioCommand <- ioOutput
	filename = filename + "x" + strconv.Itoa(turn)
	c.ioFilename <- filename

	for y := 0; y < len(World); y++ {
		for x := 0; x < len(World[0]); x++ {
			c.ioOutput <- World[y][x]
		}
	}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	c.ioCommand <- ioCheckIdle
	c.events <- StateChange{turn, Quitting}
	close(c.events)
}

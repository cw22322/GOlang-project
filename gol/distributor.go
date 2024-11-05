package gol

import (
	"fmt"
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
	keyPresses <-chan rune // Add this line to include keyPresses channel
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
	paused := false
	quitting := false
	var mu sync.Mutex

	// Initialize the world
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

	stateChan := make(chan State, 1)
	c.events <- StateChange{turn, Executing}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case key := <-c.keyPresses:
				switch key {
				case 'p':
					mu.Lock()
					paused = !paused
					if paused {
						c.events <- StateChange{turn, Paused}
					} else {
						stateChan <- Executing
						c.events <- StateChange{turn, Executing}
					}
					mu.Unlock()
				case 's':
					// Save the current state as a PGM image
					mu.Lock()
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle
					c.ioCommand <- ioOutput
					outputFilename := fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, turn)
					c.ioFilename <- outputFilename

					for y := 0; y < len(World); y++ {
						for x := 0; x < len(World[0]); x++ {
							c.ioOutput <- World[y][x]
						}
					}
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle
					mu.Unlock()
					c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: outputFilename}
				case 'q':
					mu.Lock()
					quitting = true
					stateChan <- Quitting
					mu.Unlock()
				}
			case <-ticker.C:
				mu.Lock()
				aliveCells := calculateAliveCells(p, World)
				c.events <- AliveCellsCount{CompletedTurns: turn, CellsCount: len(aliveCells)}
				mu.Unlock()
			}
		}
	}()

	for turn < p.Turns {
		// Proceed with the simulation for the current turn
		mu.Lock()
		World = calculateNextState(p, World)
		pausedCopy := paused
		quittingCopy := quitting
		turn++
		mu.Unlock()

		// Send TurnComplete event
		c.events <- TurnComplete{CompletedTurns: turn}

		if pausedCopy {
			state := <-stateChan
			if state == Quitting {
				break
			}
		}
		// Check if quitting flag is set after completing the turn
		if quittingCopy {
			break
		}
	}

	ticker.Stop()

	// Handle quitting: final events, save state, cleanup
	// Complete the current turn if not already completed
	alive := calculateAliveCells(p, World)
	c.events <- FinalTurnComplete{
		CompletedTurns: turn,
		Alive:          alive,
	}

	// Save the final state
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	c.ioCommand <- ioOutput
	outputFilename := fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, turn)
	c.ioFilename <- outputFilename

	for y := 0; y < len(World); y++ {
		for x := 0; x < len(World[0]); x++ {
			c.ioOutput <- World[y][x]
		}
	}
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle // Wait for IO to finish

	c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: outputFilename}

	// Send StateChange event indicating quitting
	c.events <- StateChange{CompletedTurns: turn, NewState: Quitting}

	// Close the channel to stop the SDL goroutine gracefully.
	close(c.events)
}

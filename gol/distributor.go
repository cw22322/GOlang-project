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

func calculateNextStateSlice(p Params, world [][]byte, startY, endY int, flippedCells chan<- []util.Cell) [][]byte {
	sliceHeight := endY - startY
	width := len(world[0])

	newSlice := make([][]byte, sliceHeight)
	for i := range newSlice {
		newSlice[i] = make([]byte, width)
	}

	height := len(world)
	var localFlipped []util.Cell

	for y := startY; y < endY; y++ {
		for x := 0; x < width; x++ {
			alive := 0

			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dy == 0 && dx == 0 {
						continue
					}

					nx, ny := (x+dx+width)%width, (y+dy+height)%height

					if world[ny][nx] == 255 {
						alive++
					}
				}
			}

			cellChanged := false
			if world[y][x] == 255 {
				if alive < 2 || alive > 3 {
					newSlice[y-startY][x] = 0
					cellChanged = true
				} else {
					newSlice[y-startY][x] = 255
				}
			} else {
				if alive == 3 {
					newSlice[y-startY][x] = 255
					cellChanged = true
				} else {
					newSlice[y-startY][x] = 0
				}
			}

			if cellChanged {
				localFlipped = append(localFlipped, util.Cell{X: x, Y: y})
			}
		}
	}

	// Send the local flipped cells back to the main thread
	flippedCells <- localFlipped

	return newSlice
}

func worker(startY, endY int, p Params, world [][]byte, out chan<- [][]byte, flippedCells chan<- []util.Cell) {
	newWorldSlice := calculateNextStateSlice(p, world, startY, endY, flippedCells)
	out <- newWorldSlice
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

	// Send CellFlipped events for initial alive cells
	initialAliveCells := calculateAliveCells(p, World)
	for _, cell := range initialAliveCells {
		c.events <- CellFlipped{CompletedTurns: turn, Cell: cell}
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
					mu.Lock()
					// Ensure IO operations are idle
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle

					// Start outputting the image
					c.ioCommand <- ioOutput
					outputFilename := fmt.Sprintf("%vx%vx%v.pgm", p.ImageWidth, p.ImageHeight, turn)
					c.ioFilename <- outputFilename

					// Send the world data
					for y := 0; y < len(World); y++ {
						for x := 0; x < len(World[0]); x++ {
							c.ioOutput <- World[y][x]
						}
					}

					// Wait for the IO to finish
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle

					mu.Unlock()

					// Send the ImageOutputComplete event with the correct filename
					c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: outputFilename}
				case 'q':
					mu.Lock()
					quitting = true
					mu.Unlock()
					// Wait for the IO to finish before quitting
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle
					stateChan <- Quitting
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
		mu.Lock()
		pausedCopy := paused
		quittingCopy := quitting
		mu.Unlock()

		if pausedCopy {
			state := <-stateChan
			if state == Quitting {
				break
			}
		}

		if quittingCopy {
			break
		}

		// Use worker functions for parallel computation
		World = parallelCalculateNextStateUsingWorkers(p, World, turn, c)

		turn++
		// Send TurnComplete event
		c.events <- TurnComplete{CompletedTurns: turn}
	}

	ticker.Stop()

	// Handle quitting: final events, save state, cleanup
	alive := calculateAliveCells(p, World)
	c.events <- FinalTurnComplete{
		CompletedTurns: turn,
		Alive:          alive,
	}

	// Save the final state
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	c.ioCommand <- ioOutput
	outputFilename := fmt.Sprintf("%vx%vx%v.pgm", p.ImageWidth, p.ImageHeight, turn)
	c.ioFilename <- outputFilename

	for y := 0; y < len(World); y++ {
		for x := 0; x < len(World[0]); x++ {
			c.ioOutput <- World[y][x]
		}
	}
	// Wait for IO to finish
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: outputFilename}

	// Send StateChange event indicating quitting
	c.events <- StateChange{CompletedTurns: turn, NewState: Quitting}

	// Close the channel to stop the SDL goroutine gracefully.
	close(c.events)
}

func parallelCalculateNextStateUsingWorkers(p Params, world [][]byte, turn int, c distributorChannels) [][]byte {
	height := len(world)
	width := len(world[0])

	newWorld := make([][]byte, height)
	for i := range newWorld {
		newWorld[i] = make([]byte, width)
	}

	numWorkers := p.Threads
	if numWorkers > height {
		numWorkers = height
	}

	var wg sync.WaitGroup
	out := make(chan WorkerResult, numWorkers)
	flippedCellsChan := make(chan []util.Cell, numWorkers)

	sliceHeight := height / numWorkers
	remainder := height % numWorkers

	startY := 0

	for i := 0; i < numWorkers; i++ {
		endY := startY + sliceHeight
		if i < remainder {
			endY++
		}

		wg.Add(1)
		go func(startY, endY int) {
			defer wg.Done()
			workerOut := make(chan [][]byte)
			go worker(startY, endY, p, world, workerOut, flippedCellsChan)
			newSlice := <-workerOut
			out <- WorkerResult{startY: startY, slice: newSlice}
		}(startY, endY)

		startY = endY
	}

	go func() {
		wg.Wait()
		close(out)
		close(flippedCellsChan)
	}()

	// Collect results from workers
	for result := range out {
		for i, row := range result.slice {
			newWorld[result.startY+i] = row
		}
	}

	// Collect flipped cells from workers
	var allFlippedCells []util.Cell
	for cells := range flippedCellsChan {
		allFlippedCells = append(allFlippedCells, cells...)
	}

	// Send CellsFlipped event if there are any flipped cells
	if len(allFlippedCells) > 0 {
		c.events <- CellsFlipped{CompletedTurns: turn + 1, Cells: allFlippedCells}
	}

	return newWorld
}

type WorkerResult struct {
	startY int
	slice  [][]byte
}

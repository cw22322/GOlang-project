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
	var aliveCells []util.Cell
	for y := 0; y < len(world); y++ {
		for x := 0; x < len(world[0]); x++ {
			if world[y][x] != 0 {
				aliveCells = append(aliveCells, util.Cell{X: x, Y: y})
			}
		}
	}
	return aliveCells
}

func calculateNextState(p Params, world [][]byte, startY, endY int) ([][]byte, []util.Cell) {
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

	return newSlice, localFlipped
}

func worker(startY, endY int, p Params, world [][]byte) ([][]byte, []util.Cell) {
	newWorldSlice, localFlipped := calculateNextState(p, world, startY, endY)
	return newWorldSlice, localFlipped
}

func distributor(p Params, c distributorChannels) {
	turn := 0
	paused := false
	quitting := false
	var mu sync.Mutex

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

	initialAliveCells := calculateAliveCells(p, World)
	for _, cell := range initialAliveCells {
		c.events <- CellFlipped{CompletedTurns: turn, Cell: cell}
	}

	stateChan := make(chan State, 1)
	c.events <- StateChange{CompletedTurns: turn, NewState: Executing}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	go func() {
		defer wg.Done()
		for {
			select {
			case key := <-c.keyPresses:
				mu.Lock()
				switch key {
				case 'p':
					paused = !paused
					if paused {
						c.events <- StateChange{CompletedTurns: turn, NewState: Paused}
					} else {
						stateChan <- Executing
						c.events <- StateChange{CompletedTurns: turn, NewState: Executing}
					}
				case 's':

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

					c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: outputFilename}
				case 'q':
					quitting = true
					c.ioCommand <- ioCheckIdle
					<-c.ioIdle
					stateChan <- Quitting
				}
				mu.Unlock()
			case <-ticker.C:
				mu.Lock()
				aliveCells := calculateAliveCells(p, World)
				c.events <- AliveCellsCount{CompletedTurns: turn, CellsCount: len(aliveCells)}
				mu.Unlock()
			case <-done:
				return
			}
		}
	}()

	for turn < p.Turns {
		mu.Lock()
		World = parallel(p, World, turn, c)
		pausedCopy := paused
		quittingCopy := quitting
		turn++
		mu.Unlock()

		c.events <- TurnComplete{CompletedTurns: turn}

		if pausedCopy {
			state := <-stateChan
			if state == Quitting {
				break
			}
		}

		if quittingCopy {
			break
		}
	}

	mu.Lock()
	alive := calculateAliveCells(p, World)
	c.events <- FinalTurnComplete{
		CompletedTurns: turn,
		Alive:          alive,
	}
	mu.Unlock()

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

	c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: outputFilename}

	c.events <- StateChange{CompletedTurns: turn, NewState: Quitting}

	close(done)

	wg.Wait()

	close(c.events)
}

func parallel(p Params, world [][]byte, turn int, c distributorChannels) [][]byte {
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
	var mu sync.Mutex
	var allFlippedCells []util.Cell

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
			newSlice, localFlipped := worker(startY, endY, p, world)

			// Protect access to newWorld and allFlippedCells with a mutex
			mu.Lock()
			for i := 0; i < endY-startY; i++ {
				newWorld[startY+i] = newSlice[i]
			}
			allFlippedCells = append(allFlippedCells, localFlipped...)
			mu.Unlock()
		}(startY, endY)

		startY = endY
	}

	wg.Wait()

	if len(allFlippedCells) > 0 {
		c.events <- CellsFlipped{CompletedTurns: turn + 1, Cells: allFlippedCells}
	}

	return newWorld
}

package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/rpc"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

type Params struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

type gameOfLife struct{}

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

func (g *gameOfLife) worker(startY, endY int, p Params, world [][]byte, out chan<- [][]byte) {
	worldSlice := world[startY:endY]

	newWorld := calculateNextState(p, worldSlice)

	out <- newWorld

	return
}

func main() {
	portAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()
	rand.Seed(time.Now().UnixNano())
	err := rpc.Register(&gameOfLife{})
	if err != nil {
		fmt.Println(err)
		return
	}
	listener, _ := net.Listen("tcp", ":"+*portAddr)
	defer func(listener net.Listener) {
		err := listener.Close()
		if err != nil {
			println(err)
		}
	}(listener)
	rpc.Accept(listener)
}

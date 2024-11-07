package main

import (
	"flag"
	"net"
	"net/rpc"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
)

type Params struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

type Request struct {
	Params Params
	World  [][]byte
	Turns  int
	StartY int
	EndY   int
}

type Response struct {
	LastWorld  [][]byte
	AliveCells []util.Cell
	Turns      int
}

type GameOfLife struct {
	world [][]byte
}

var turn int
var mu = sync.Mutex{}

func (g *GameOfLife) CalculateNextState(req Request, res *Response) (err error) {
	p := req.Params
	world := req.World
	startY := req.StartY
	endY := req.EndY

	sliceHeight := endY - startY
	sliceWidth := len(world[0])

	newSlice := make([][]byte, sliceHeight)
	for i := range newSlice {
		newSlice[i] = make([]byte, sliceWidth)
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
						ny = len(newSlice) - 1
					}

					if nx >= len(world[0]) {
						nx = 0
					}

					if ny >= len(newSlice) {
						ny = 0
					}

					if world[ny][nx] == 255 {
						alive++
					}

				}

			}
			if world[y][x] == 255 {
				if alive < 2 {
					newSlice[y][x] = 0
				} else if alive == 2 || alive == 3 {
					newSlice[y][x] = 255
				} else {
					newSlice[y][x] = 0
				}
			} else {
				if alive == 3 {
					newSlice[y][x] = 255

				} else {
					newSlice[y][x] = 0
				}
			}
		}
	}
	res.LastWorld = newSlice
	return
}

func main() {
	portAddr := flag.String("port", "8031", "Port to listen on")
	flag.Parse()
	game := new(GameOfLife)
	rpc.Register(game)
	listener, _ := net.Listen("tcp", ":"+*portAddr)
	defer listener.Close()

	rpc.Accept(listener)

}

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

type Request struct {
	Params Params
	World  [][]byte
	Turns  int
}

type Response struct {
	LastWorld [][]byte
}

type GameOfLife struct{}

func (g *GameOfLife) ProcessTurns(req Request, res *Response) error {
	world := req.World
	for turn := 0; turn < req.Turns; turn++ {
		world = calculateNextState(req.Params, world)
	}
	res.LastWorld = world
	return nil
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

func main() {
	portAddr := flag.String("port", "8031", "Port to listen on")
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	game := new(GameOfLife)
	err := rpc.Register(game)
	if err != nil {
		fmt.Println("Error registering RPC:", err)
		return
	}

	listener, err := net.Listen("tcp", ":"+*portAddr)
	if err != nil {
		fmt.Println("Error starting listener:", err)
		return
	}
	defer listener.Close()
	fmt.Println("Server listening on port:", *portAddr)

	go func() {
		rpc.Accept(listener)
	}()

	select {}
}

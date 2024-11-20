package main

import (
	"flag"
	"net"
	"net/rpc"
	"os"
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
	turn  int
	mu    sync.Mutex
}

var end = make(chan bool)

func (g *GameOfLife) Kill(req Request, res *Response) (err error) {
	end <- true
	return
}

func (g *GameOfLife) CalculateNextState(req Request, res *Response) error {
	world := req.World
	p := req.Params
	newWorld := make([][]byte, len(world))
	for i := range newWorld {
		newWorld[i] = make([]byte, p.ImageWidth)
	}

	for y := 1; y < len(world)-1; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			alive := 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dy == 0 && dx == 0 {
						continue
					}
					ny := y + dy
					nx := (x + dx + p.ImageWidth) % p.ImageWidth
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

	res.LastWorld = newWorld
	return nil
}

func main() {
	go func() {
		for {
			if <-end {
				os.Exit(1)
			}
		}
	}()
	portAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()
	game := new(GameOfLife)
	rpc.Register(game)
	listener, _ := net.Listen("tcp", ":"+*portAddr)
	defer listener.Close()

	rpc.Accept(listener)

}

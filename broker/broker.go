package main

import (
	"flag"
	"net"
	"net/rpc"
	"os"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
)

var Ports = [4]string{"54.88.110.115:8030", "54.90.136.10:8030", "54.90.238.159:8030", "54.82.22.143:8030"}

type Params struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
	StartY      int
	EndY        int
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
	world               [][]byte
	turn                int
	mu                  sync.Mutex
	paused              bool
	controllerConnected bool
}

func calculateAliveCells(world [][]byte) []util.Cell {
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

func worker(startY, endY int, p Params, world [][]byte, out chan<- [][]byte, port string) {
	numRows := endY - startY + 3
	worldToSend := make([][]byte, 0, numRows)

	for y := startY - 1; y <= endY+1; y++ {
		ny := (y + p.ImageHeight) % p.ImageHeight
		worldToSend = append(worldToSend, world[ny])
	}

	request := Request{
		Params: p,
		World:  worldToSend,
	}

	var res Response
	serverAddr := port
	client, _ := rpc.Dial("tcp", serverAddr)
	defer client.Close()
	client.Call("GameOfLife.CalculateNextState", request, &res)
	out <- res.LastWorld
}

func (g *GameOfLife) GetInfo(req Request, res *Response) (err error) {
	res.LastWorld = g.world
	res.Turns = g.turn
	res.AliveCells = calculateAliveCells(g.world)
	return
}

func (g *GameOfLife) Pause(req Request, res *Response) (err error) {
	return
}

func (g *GameOfLife) Unpause(req Request, res *Response) (err error) {
	return
}

var end = make(chan bool)

func (g *GameOfLife) Kill(req Request, res *Response) (err error) {
	end <- true
	return
}

func (g *GameOfLife) ProcessTurns(req Request, res *Response) error {
	g.mu.Lock()
	g.world = req.World
	g.mu.Unlock()
	p := req.Params
	workerCount := 4
	workerHeight := p.ImageHeight / workerCount

	for i := 0; i < p.Turns; i++ {
		out := make([]chan [][]byte, workerCount)
		for j := 0; j < workerCount; j++ {
			out[j] = make(chan [][]byte)
		}

		for j := 0; j < workerCount; j++ {
			startY := j * workerHeight
			endY := startY + workerHeight - 1
			if j == workerCount-1 {
				endY = p.ImageHeight - 1
			}
			go worker(startY, endY, p, g.world, out[j], Ports[j])
		}

		newWorld := make([][]byte, 0, p.ImageHeight)

		for j := 0; j < workerCount; j++ {
			workerResult := <-out[j]
			workerRows := workerResult[1 : len(workerResult)-1]
			newWorld = append(newWorld, workerRows...)
		}
		g.mu.Lock()
		g.turn = i + 1
		g.world = newWorld
		g.mu.Unlock()
	}

	g.mu.Lock()
	res.LastWorld = g.world
	res.AliveCells = calculateAliveCells(g.world)
	res.Turns = g.turn
	g.mu.Unlock()

	return nil
}

func (g *GameOfLife) SendAlive(req Request, res *Response) (err error) {
	g.mu.Lock()
	alive := calculateAliveCells(g.world)
	res.AliveCells = alive
	res.Turns = g.turn
	g.mu.Unlock()

	return
}

func (g *GameOfLife) Save(req Request, res *Response) (err error) {
	g.mu.Lock()
	res.LastWorld = g.world
	g.mu.Unlock()
	return
}

func main() {

	go func() {
		for {
			if <-end {
				for i := 0; i < 4; i++ {
					serverAddr := Ports[i]
					client, _ := rpc.Dial("tcp", serverAddr)
					defer client.Close()
					client.Call("GameOfLife.Kill", nil, nil)
				}
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

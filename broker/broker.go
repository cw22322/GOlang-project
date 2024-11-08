package main

import (
	"flag"
	"net"
	"net/rpc"
	"strconv"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
)

var Ports = [2]int{8031, 8032}

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
	world [][]byte
	turn  int
	mu    sync.Mutex
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

func worker(startY, endY int, p Params, world [][]byte, out chan<- [][]byte, port int) {
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
	serverAddr := "127.0.0.1:" + strconv.Itoa(port)
	client, _ := rpc.Dial("tcp", serverAddr)
	defer client.Close()
	client.Call("GameOfLife.CalculateNextState", request, &res)
	out <- res.LastWorld
}

func (g *GameOfLife) ProcessTurns(req Request, res *Response) error {
	g.mu.Lock()
	g.world = req.World
	g.mu.Unlock()
	p := req.Params
	workerCount := 2
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
	portAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()
	game := new(GameOfLife)
	rpc.Register(game)
	listener, _ := net.Listen("tcp", ":"+*portAddr)
	defer listener.Close()

	rpc.Accept(listener)

}

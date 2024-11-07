package main

import (
	"flag"
	"net"
	"net/rpc"
	"strconv"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
)

var mu = sync.Mutex{}
var Ports = [4]int{8031, 8032}

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
	numRows := endY - startY + 3 //
	worldToSend := make([][]byte, numRows)

	index := 0
	for y := startY - 1; y <= endY+1; y++ {
		ny := (y + p.ImageHeight) % p.ImageHeight
		worldToSend[index] = world[ny]
		index++
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
	g.world = req.World
	p := req.Params
	workerCount := 2
	workerHeight := p.ImageHeight / workerCount

	for turn := 0; turn < p.Turns; turn++ {
		out := make([]chan [][]byte, workerCount)
		for i := 0; i < workerCount; i++ {
			out[i] = make(chan [][]byte)
		}

		for i := 0; i < workerCount; i++ {
			startY := i * workerHeight
			endY := startY + workerHeight - 1
			if i == workerCount-1 {
				endY = p.ImageHeight - 1
			}
			go worker(startY, endY, p, g.world, out[i], Ports[i])
		}

		newWorld := make([][]byte, p.ImageHeight)
		index := 0
		for i := 0; i < workerCount; i++ {
			workerResult := <-out[i]
			workerRows := workerResult[1 : len(workerResult)-1]
			copy(newWorld[index:index+len(workerRows)], workerRows)
			index += len(workerRows)
		}
		g.world = newWorld
	}

	res.LastWorld = g.world
	res.AliveCells = calculateAliveCells(g.world)
	res.Turns = p.Turns

	return nil
}

func (g *GameOfLife) SendAlive(req Request, res *Response) (err error) {
	p := req.Params
	world := req.World
	alive := calculateAliveCells(world)
	res.AliveCells = alive
	res.Turns = p.Turns

	return
}

func (g *GameOfLife) Save(req Request, res *Response) (err error) {
	mu.Lock()
	res.LastWorld = g.world
	mu.Unlock()
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

package main

import (
	"flag"
	"net"
	"net/rpc"
	"strconv"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
)

var turn int
var mu = sync.Mutex{}
var Ports = [4]int{8031, 8032, 8033, 8034}

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
	var worldToSend = make([][]byte, 0)
	for y := startY; y <= endY; y++ {
		ny := (y + p.ImageHeight) % p.ImageHeight
		worldToSend[y-startY] = world[ny]
	}

	request := Request{
		Params: p,
		World:  worldToSend,
	}
	var res Response
	serverAddr := "127.0.0.1:" + strconv.Itoa(port)
	client, _ := rpc.Dial("tcp", serverAddr)
	client.Call("GameOfLife.CalculateNextState", request, &res)
	defer client.Close()
	world = res.LastWorld
	out <- world
}

func (g *GameOfLife) ProcessTurns(req Request, res *Response) (err error) {
	turn = 0
	g.world = req.World
	p := req.Params
	workerHeight := p.ImageHeight / 2

	for turn := 0; turn < p.Turns; turn++ {
		out := make([]chan [][]byte, 4)
		for i := 0; i < 4; i++ {
			out[i] = make(chan [][]byte)
		}

		for i := 0; i < 4; i++ {
			startY := (i * workerHeight) % p.ImageHeight
			endY := (startY + workerHeight - 1) % p.ImageHeight
			if i == 3 {
				endY = p.ImageHeight - 1
			}
			go worker(startY-1, endY+1, p, g.world, out[i], Ports[i])
		}
		newWorld := make([][]byte, p.ImageHeight)
		index := 0
		for i := 0; i < 4; i++ {
			workerResult := <-out[i]
			workerHeight := len(workerResult) - 2
			copy(newWorld[index:index+workerHeight], workerResult[1:len(workerResult)-1])
			index += workerHeight
		}
		g.world = newWorld
	}
	res.LastWorld = g.world
	res.AliveCells = calculateAliveCells(g.world)
	res.Turns = p.Turns

	return
}

func (g *GameOfLife) SendAlive(req Request, res *Response) (err error) {
	mu.Lock()
	alive := calculateAliveCells(g.world)
	res.AliveCells = alive
	res.Turns = turn
	mu.Unlock()

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

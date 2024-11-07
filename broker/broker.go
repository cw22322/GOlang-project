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

func worker(startY, endY int, p Params, world [][]byte, out chan<- [][]byte) {
	for _, element := range Ports {

		var worldToSend = make([][]byte, 0)
		for y := startY; y < endY; y++ {
			ny := y
			if ny < 0 {
				ny = len(world) - 1
			}
			if ny >= len(world) {
				ny = 0
			}
			worldToSend = append(worldToSend, world[ny])
		}

		request := Request{
			Params: p,
			World:  worldToSend,
		}
		var res Response
		serverAddr := "127.0.0.1:" + strconv.Itoa(element)
		client, _ := rpc.Dial("tcp", serverAddr)
		client.Call("GameOfLife.CalculateNextState", request, &res)
		world = res.LastWorld
		out <- world
	}
}

func (g *GameOfLife) ProcessTurns(req Request, res *Response) (err error) {
	turn = 0
	g.world = req.World
	p := req.Params
	workerHeight := p.ImageHeight / 4
	out := make([]chan [][]byte, 4)
	for i := range out {
		out[i] = make(chan [][]byte)
	}
	for turn < req.Turns {
		mu.Lock()
		for i := 0; i < 4; i++ {
			go worker(i*workerHeight-1, (i+1)*workerHeight+1, p, g.world, out[i])
		}
		turn++
		mu.Unlock()
	}
	g.world = make([][]byte, 0)
	for _, result := range out {
		g.world = append(g.world, <-result...)
	}
	res.AliveCells = calculateAliveCells(g.world)
	res.LastWorld = g.world
	res.Turns = turn

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

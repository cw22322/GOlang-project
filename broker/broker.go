package main

import (
	"flag"
	"net"
	"net/rpc"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
)

var turn int
var mu = sync.Mutex{}

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
	LastWorld  [][]byte
	AliveCells []util.Cell
	Turns      int
}

type GameOfLife struct {
	world [][]byte
}

func calculateAliveCells(p Params, world [][]byte) []util.Cell {
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

func (g *GameOfLife) ProcessTurns(req Request, res *Response) (err error) {
	turn = 0
	serverAddr := "127.0.0.1:8031"
	client, _ := rpc.Dial("tcp", serverAddr)
	g.world = req.World
	for turn < req.Turns {
		mu.Lock()
		var request Request
		request.World = g.world
		request.Params = req.Params
		var response Response
		client.Call("GameOfLife.CalculateNextState", request, response)
		g.world = response.LastWorld
		turn++
		mu.Unlock()
	}
	res.AliveCells = calculateAliveCells(req.Params, g.world)
	res.LastWorld = g.world

	return
}

func (g *GameOfLife) SendAlive(req Request, res *Response) (err error) {
	mu.Lock()
	alive := calculateAliveCells(req.Params, g.world)
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

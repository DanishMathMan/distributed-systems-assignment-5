package client

import (
	"distributed-systems-assignment-5/src/utility/connections"
	"distributed-systems-assignment-5/src/utility/lamportClock"
)

type AuctionClient struct {
	ClientId      int64
	ServerPool    map[int64]connections.ClientConnection
	CurrentLeader connections.ClientConnection
	LamportClock  lamportClock.LamportClock
}

func CreateAuctionClient(id int64) AuctionClient {
	client := new(AuctionClient)
	client.ClientId = id
	client.LamportClock = lamportClock.CreateLamportClock()
	client.ServerPool = make(map[int64]connections.ClientConnection)
	client.CurrentLeader = client.ServerPool[client.ClientId]
	return *client
}

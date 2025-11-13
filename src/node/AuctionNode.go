package node

import (
	proto "distributed-systems-assignment-5/src/grpc"
	"distributed-systems-assignment-5/src/utility/connections"
	"distributed-systems-assignment-5/src/utility/lamportClock"
)

type AuctionNode struct {
	proto.UnimplementedNodeServer
	Port         int64
	NodeId       int64
	LamportClock lamportClock.LamportClock
	ClientPool   map[int64]connections.ClientConnection //all clients which are connected to it
	ServerPool   map[int64]connections.ServerConnection //all other nodes
	//TODO ? LeaderNodeId int64
	BidQueue BidQueue //queue of not-yet handled Bid rpc requests
}

func CreateAuctionNode(port int64, id int64) AuctionNode {
	node := new(AuctionNode)
	node.Port = port
	node.NodeId = id
	node.LamportClock = lamportClock.CreateLamportClock()
	node.BidQueue = BidQueue{}
	node.ClientPool = make(map[int64]connections.ClientConnection)
	node.ServerPool = make(map[int64]connections.ServerConnection)
	return *node
}

package node

import (
	proto "distributed-systems-assignment-5/src/grpc"
	"distributed-systems-assignment-5/src/utility/lamportClock"
)

type AuctionNode struct {
	proto.UnimplementedNodeServer
	Port         int64
	NodeId       int64
	LamportClock lamportClock.LamportClock
	//todo not sure how it should communicate with other serversBackupServers   map[int64]connections.ClientConnection //connections it has to backup servers
	//todo CurrentLeader
	//todo should it explicitly know about its clients?
	BidQueue BidQueue //queue of not-yet handled Bid rpc requests
}

func CreateAuctionNode(port int64, id int64) AuctionNode {
	node := new(AuctionNode)
	node.Port = port
	node.NodeId = id
	node.LamportClock = lamportClock.CreateLamportClock()
	node.BidQueue = BidQueue{}
	//node.BackupServers = make(map[int64]connections.ClientConnection)
	return *node
}

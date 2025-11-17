package main

import (
	proto "distributed-systems-assignment-5/src/grpc"
	"distributed-systems-assignment-5/src/utility"
	"distributed-systems-assignment-5/src/utility/lamportClock"
	"flag"
	"net"
	"os"
	"strconv"

	"google.golang.org/grpc"
)

type AuctionNode struct {
	proto.UnimplementedNodeServer
	Port         int64
	NodeId       int64
	LamportClock lamportClock.LamportClock
	//todo not sure how it should communicate with other serversBackupServers   map[int64]connections.ClientConnection //connections it has to backup servers
	//todo CurrentLeader
	//todo should it explicitly know about its clients?
	BidQueue utility.BidQueue //queue of not-yet handled Bid rpc requests
}

func CreateAuctionNode(port int64, id int64) AuctionNode {
	node := new(AuctionNode)
	node.Port = port
	node.NodeId = id
	node.LamportClock = lamportClock.CreateLamportClock()
	node.BidQueue = utility.BidQueue{}
	//node.BackupServers = make(map[int64]connections.ClientConnection)
	return *node
}

func main() {
	port := flag.Int64("port", 8080, "Input port for the server to start on. Note, port is also its id")
	id := flag.Int64("id", 0, "Auction node id")
	flag.Parse()
	server := CreateAuctionNode(*port, *id)
	server.StartServer()

	//TODO after an X amount of time, the leader must inicate the auction has stopped,
	//broadcast to all clients the highest bid
}

func (node *AuctionNode) NormalOperation() {
	//TODO
	//node.Request()
	//node.Coordination()
	//node.Execution()
	//node.Agreement()
	//node.Response()
}

func (node *AuctionNode) StartServer() {
	grpcServer := grpc.NewServer()
	lis, err := net.Listen("tcp", ":"+strconv.FormatInt(node.Port, 10))
	if err != nil {
		os.Exit(1)
	}
	proto.RegisterNodeServer(grpcServer, node)
	err = grpcServer.Serve(lis)
	if err != nil {
		os.Exit(1)
	}
}

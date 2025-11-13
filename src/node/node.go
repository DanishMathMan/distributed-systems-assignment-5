package node

import (
	proto "distributed-systems-assignment-5/src/grpc"
	"flag"
	"net"
	"os"
	"strconv"

	"google.golang.org/grpc"
)

func main() {
	port := flag.Int64("port", 8080, "Input port for the server to start on. Note, port is also its id")
	id := flag.Int64("id", 0, "Auction node id")
	flag.Parse()
	server := CreateAuctionNode(*port, *id)
	server.StartServer()
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

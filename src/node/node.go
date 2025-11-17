package main

import (
	"bufio"
	"context"
	proto "distributed-systems-assignment-5/src/grpc"
	"distributed-systems-assignment-5/src/utility"
	"distributed-systems-assignment-5/src/utility/connections"
	"distributed-systems-assignment-5/src/utility/lamportClock"
	"errors"
	"flag"
	"net"
	"os"
	"regexp"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type AuctionNode struct {
	proto.UnimplementedNodeServer
	Port         int64
	NodeId       int64
	LamportClock lamportClock.LamportClock
	OutClients   map[int64]connections.ClientConnection //connections it has to backup servers
	//Replies      map[int64]chan bool
	Leader int64
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

// NormalOperation must be run when the primary node receives rpc calls from clients
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

func (node *AuctionNode) InputHandler() error {
	regex := regexp.MustCompile("(?:--connect|-c) *(?P<port>\\d{4})") //expect a four digit port number
	regex2 := regexp.MustCompile("--start")

	for {
		reader := bufio.NewReader(os.Stdin)
		msg, err := reader.ReadString('\n')
		if err != nil {
			//utility.LogAsJson(utility.LogStruct{Timestamp: node.GetTimestamp(), Identifier: node.PortId, Message: err.Error()}, true)
			continue
		}
		match := regex.FindStringSubmatch(msg)
		match2 := regex2.FindStringSubmatch(msg)
		if match == nil && match2 == nil {
			//utility.LogAsJson(utility.LogStruct{Timestamp: node.GetTimestamp(), Identifier: node.PortId, Message: "Invalid input: " + strings.Split(msg, "\n")[0]}, true)
			continue
		}

		//todo starter flag to start the servers
		//Starter flag
		/*
			if match2 != nil {
				node.Start <- true
				continue
			}
		*/

		port, _ := strconv.ParseInt(match[1], 10, 64)
		err = node.ConnectToNodeServer(port)
		if err != nil {
			//utility.LogAsJson(utility.LogStruct{Timestamp: node.GetTimestamp(), Identifier: node.PortId, Message: err.Error()}, true)
			continue
		}
	}
}

func (node *AuctionNode) ConnectToNodeServer(port int64) error {
	if node.OutClients[port].Client != nil {
		return errors.New("Already connected to port: " + strconv.FormatInt(port, 10))
	}

	node.LamportClock.LocalEvent()
	conn, err := grpc.NewClient("localhost:"+strconv.FormatInt(port, 10), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	client := proto.NewNodeClient(conn)
	_, err = client.Ping(context.Background(), &proto.Empty{})
	if err != nil {
		return err
	}
	node.OutClients[port] = connections.ClientConnection{Client: client, IsDown: false}
	//node.Replies[port] = make(chan bool, 1)

	return nil
}

// TODO BULLY

// Election is the logic the server must run when it receives an Election rpc call from another server.
func (node *AuctionNode) Election(ctx context.Context, empty *proto.Empty) (*proto.Empty, error) {
	/*
	   On pi receive ‘election’ do
	      	if i == max process id // i am the bully
	      		send ‘coordinator(i)
	   	end if

	      for all (j > i)
	      	send ‘election to pj’, timeout c
	      	on timeout: // all bullies are dead 😱
	      			for all ( j <i)
	      				send ‘coordinator(i)
	      			end for
	      		end on
	      	end for

	      end on
	*/

	if node.Leader == node.NodeId {
		for _, conn := range node.OutClients {
			_, err := conn.Client.Coordinator(nil, &proto.CoordinatorMessage{NodeId: node.NodeId})
			if err != nil {
				//TODO handle rpc error. This probably means that the other server is down
			}
		}
	}

	//todo Det skal nok forstås som en timeout for alle beskederne, frem for kun 1 - Mads TA

	//because go doesn't have while/for loops with conditionals we do the following for the statement: "for all (j > i)"
	for conn_id, conn := range node.OutClients {
		//skip all nodes that can't be its bully
		if conn_id <= node.NodeId {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*2))
		defer cancel()
		_, err := conn.Client.Election(ctx, nil)
		if err != nil {
			//WithTimeout error can be checked with status.Code(err) == codes.DeadlineExceeded
			//todo consider removing connection from pool
			for conn_id2, conn2 := range node.OutClients {
				if conn_id2 < node.NodeId {
					conn2.Client.Coordinator(nil, &proto.CoordinatorMessage{NodeId: node.NodeId})
				}
			}
		}
	}
	return nil, nil
}

// Coordinator is the logic the server must run when it receives a Coordinator rpc call from another server.
func (node *AuctionNode) Coordinator(ctx context.Context, msg *proto.CoordinatorMessage) (*proto.Empty, error) {
	//receiving a Coordinator call, means that the "bully"/leader has been decided and is the one who made the call
	node.Leader = msg.NodeId
	return nil, nil
}

// Answer is the logic the server must run when it receives an Answer rpc call from another server.
func (node *AuctionNode) Answer(ctx context.Context, empty *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

// Ping is the logic the server must run when it receives a Ping rpc call from another server.
func (node *AuctionNode) Ping(ctx context.Context, empty *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

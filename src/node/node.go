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
	"sync"

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
	BidQueue            utility.BidQueue //queue of not-yet handled Bid rpc requests
	HasStartedAnElected bool
}

func CreateAuctionNode(port int64, id int64) AuctionNode {
	node := new(AuctionNode)
	node.Port = port
	node.NodeId = id
	node.LamportClock = lamportClock.CreateLamportClock()
	node.BidQueue = utility.BidQueue{}
	node.HasStartedAnElected = false
	//node.BackupServers = make(map[int64]connections.ClientConnection)
	return *node
}

func main() {
	port := flag.Int64("port", 8080, "Input port for the server to start on. Note, port is also its id")
	flag.Parse()
	server := CreateAuctionNode(*port, *port)
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
	answerChannel := make(chan bool, 1)
	connection := connections.ClientConnection{Client: client, IsDown: false, Answer: answerChannel}
	node.OutClients[port] = connection
	//node.Replies[port] = make(chan bool, 1)

	return nil
}

// TODO BULLY

// CallElection uses the bully algorihtm to determine a leader.
// Based on the algorithm as described on https://en.wikipedia.org/wiki/Bully_algorithm last accessed 20/11/2025 TODO update date if necessary
func (node *AuctionNode) CallElection() {
	/*
		    1. If P has the highest process ID, it sends a Victory message to all other processes and becomes the new Coordinator. Otherwise, P broadcasts an Election message to all other processes with higher process IDs than itself.
			2. If P receives no Answer after sending an Election message, then it broadcasts a Victory message to all other processes and becomes the Coordinator.
			3. If P receives an Answer from a process with a higher ID, it sends no further messages for this election and waits for a Victory message. (If there is no Victory message after a period of time, it restarts the process at the beginning.)
			4. If P receives an Election message from another process with a lower ID it sends an Answer message back and if it has not already started an election, it starts the election process at the beginning, by sending an Election message to higher-numbered processes.
			5. If P receives a Coordinator message, it treats the sender as the coordinator.
	*/

	node.HasStartedAnElected = true
	IsHighest := true
	for conn_id, _ := range node.OutClients {
		if conn_id > node.NodeId {
			//it is not the highest
			IsHighest = false
			break
		}
	}
	//the value of IsHighest will now reflect whether it is highest or not
	if IsHighest {
		for _, c := range node.OutClients {
			//TODO TIMEOUT
			_, err := c.Client.Coordinator(nil, &proto.CoordinatorMessage{NodeId: node.NodeId})
			if err != nil {
				//TODO HANDLE CORRECTLY
				c.IsDown = true
			}

		}
		node.Leader = node.NodeId
	} else {
		higherNodesCount := 0 //count the number of nodes that are higher than it which it knows about
		failedNodesCount := 0 //count the number of those nodes that fail to respond
		for v, c := range node.OutClients {
			//only broadcast to every node which is higher than itself
			if v < node.NodeId {
				continue
			}
			higherNodesCount++
			timestamp := node.LamportClock.LocalEvent()
			electionMsg := proto.ElectionMessage{Timestamp: timestamp, NodeId: node.NodeId}
			wg := sync.WaitGroup{}
			wg.Go(func() {
				//TODO TIMEOUT
				_, err := c.Client.Election(nil, &electionMsg)
				if err != nil {
					//TODO HANDLE CORRECTLY
					c.IsDown = true
					failedNodesCount++
				}
			})
			// The node received an answer, therefore it must wait for receiving a Coordinator rpc call or timeout, whichever comes first
			//TODO HANDLE RECEIVING AN ANSWER PER STEP 3 AND 4
			//TODO WAIT FOR A VICTORY CALL OR TIMEOUT

			//map over clients, hver client har et ID og en channel
			//indhent og afvent alle channels fra de højere id's
			//Hvis en af channels bliver populated stop alle andre channel listeners samt funktionen.

		}
		//check if all the nodes that are higher than it failed. If they did this is now the leader
		if higherNodesCount == failedNodesCount {
			//no other node that is higher than this node are active and thus this node becomes the leader
			for _, c := range node.OutClients {
				//TODO TIMEOUT
				_, err := c.Client.Coordinator(nil, &proto.CoordinatorMessage{NodeId: node.NodeId})
				if err != nil {
					//TODO HANDLE CORRECTLY
				}
			}
		} else {
			// The node received an answer, therefore it must wait for receiving a Coordinator rpc call or timeout, whichever comes first
			//TODO HANDLE RECEIVING AN ANSWER PER STEP 3 AND 4
			//TODO WAIT FOR A VICTORY CALL OR TIMEOUT

		}
	}

	defer func() {
		node.HasStartedAnElected = false
	}()
}

// Election is the logic the server must run when it receives an Election rpc call from another server.
func (node *AuctionNode) Election(ctx context.Context, electionMsg *proto.ElectionMessage) (*proto.Empty, error) {

	timestamp := node.LamportClock.RemoteEvent(electionMsg.Timestamp)
	if node.NodeId <= electionMsg.NodeId {
		//TODO THROW AN ERROR
	}
	_, err := node.OutClients[electionMsg.GetNodeId()].Client.Answer(nil, &proto.AnswerMessage{NodeId: node.NodeId, Timestamp: timestamp})
	if err != nil {
		//TODO HANDLE
	}
	if node.HasStartedAnElected == false {
		node.CallElection()
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
func (node *AuctionNode) Answer(ctx context.Context, answer *proto.AnswerMessage) (*proto.Empty, error) {
	if answer.NodeId <= node.NodeId {
		//TODO THROW AN ERROR
	}

	//populate channel indicating an answer for use in the CallElection method
	node.OutClients[answer.NodeId].Answer <- true
	return &proto.Empty{}, nil
}

// Ping is the logic the server must run when it receives a Ping rpc call from another server.
func (node *AuctionNode) Ping(ctx context.Context, empty *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

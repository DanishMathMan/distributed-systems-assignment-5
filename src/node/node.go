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
	"fmt"
	"log"
	"net"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type AuctionNode struct {
	proto.UnimplementedNodeServer
	Port         int64
	NodeId       int64
	LamportClock lamportClock.LamportClock
	OutClients   map[int64]connections.ClientConnection //connections it has to backup servers
	InClients    map[int64]chan interface{}             //connections from AuctionClients with channels of their messages to respond with
	//Replies      map[int64]chan bool
	AllFailed           chan bool
	ReceivedAnswer      chan bool
	NewLeader           chan bool
	StartFlag           chan bool
	Leader              int64
	RequestQueue        utility.RequestQueue //queue of not-yet handled Bid rpc requests
	Bids                map[int64]*proto.BidMessage
	BestBid             *proto.BidMessage //todo consider mutex locking to avoid race condition
	BidLock             *sync.Mutex
	HasStartedAnElected bool
	AuctionIsOver       bool
}

func CreateAuctionNode(id int64, port int64, leaderId int64) AuctionNode {
	node := new(AuctionNode)
	node.Port = port
	node.NodeId = id
	node.LamportClock = lamportClock.CreateLamportClock()
	node.OutClients = make(map[int64]connections.ClientConnection)
	node.InClients = make(map[int64]chan interface{}, 1)
	node.Leader = leaderId
	node.RequestQueue = utility.RequestQueue{}
	node.BestBid = &proto.BidMessage{Timestamp: 0, BidderId: -1, Amount: 0}
	node.Bids = make(map[int64]*proto.BidMessage)
	node.HasStartedAnElected = false
	node.AllFailed = make(chan bool, 1)
	node.ReceivedAnswer = make(chan bool, 1)
	node.NewLeader = make(chan bool, 1)
	node.StartFlag = make(chan bool)
	node.AuctionIsOver = false
	return *node
}

func main() {
	port := flag.Int64("port", 8080, "Input port for the server to start on. Note, port is also its id")
	leaderId := flag.Int64("leader", 0, "Start leader for the server")
	id := port
	flag.Parse()
	//Create server with id, port, leader's id (which is the port of the leader server).
	server := CreateAuctionNode(*id, *port, *leaderId)
	fmt.Println("Started server on port ", *port, " with ID: ", *id)
	go server.StartServer()

	//Handles input:
	// --connection (port) | Establish connection to another server
	// --start | Starts the actual server
	go func() {
		err := server.InputHandler()
		if err != nil {
			//TODO HANDLE
		}
	}()

	//wait for starter flag to have been called
	fmt.Println("Waiting for start flag ", *port)
	<-server.StartFlag
	fmt.Println("Got the start flag, starting on: ", *port)

	//continuously make sure the node has a connection to the leader
	go func() {
		for {
			//We are already leader, no need to ping
			if server.Leader == server.NodeId {
				fmt.Println("Leader ID: ", server.Leader, " Server ID: ", server.NodeId)
				fmt.Println("Sleeping as leader")
				time.Sleep(1 * time.Second)
				continue
			}

			time.Sleep(1 * time.Second)
			fmt.Println("Sleeping as normal node")

			//Ping the current leader
			//even if the call times out and the leader is still alive (but slow), the election would just reelect the leader
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			//Ensure we have a client for the leader connection.
			leaderClient := server.OutClients[server.Leader].Client

			//If our leader client is nil we are currently updating it.
			if leaderClient == nil {
				fmt.Println("Leader's client is now dead / closed.")
				continue
			}

			//Ping the leaders client, if we dont get a response then
			_, err := leaderClient.Ping(ctx, nil)
			fmt.Println("Pinging leader ", server.Leader)
			if err != nil {

				if status.Code(err) == codes.Unavailable || strings.Contains(err.Error(), "EOF") {
					//Node is dead
					fmt.Println("The leader is dead!")
					delete(server.OutClients, server.Leader)
					fmt.Println("Deleted leader ", server.Leader)
				}

				//The leader crashed, call an election
				fmt.Println("Called an election!")
				server.CallElection()
			}
		}

	}()

	//continuously handle elements in queue TODO should it matter if it is a leader or not with respect to what it does?
	go func() {
		for {

			//If no requests in queue, continue.
			if server.RequestQueue.IsEmpty() {
				continue
			}
			//Dequeue the request
			request := server.RequestQueue.Dequeue()

			switch msg := request.(type) {
			case *proto.BidMessage:

				fmt.Println("Recieved bid message: ", msg.BidderId)

				//if the node is not leader, send an exception alongside information regarding which port is the leader
				if server.Leader != server.NodeId {

					fmt.Println("Received a message as an backup node")

					if server.InClients[msg.BidderId] == nil {
						fmt.Println("Bidder ID was not found! (at line 192)")
					}

					fmt.Println("Sending ack now to check for leader matchup: ", msg.BidderId)

					server.InClients[msg.BidderId] <- &proto.Ack{
						Timestamp:         server.LamportClock.LocalEvent(),
						Response:          int32(utility.EXCEPTION),
						CurrentLeaderPort: server.Leader,
					}
					continue
				} else {
					//Node is the leader. Handle the bid.
					var response int32
					if msg.Amount <= server.BestBid.Amount {
						response = int32(utility.FAILURE)
					} else {
						response = int32(utility.SUCCESS)
						server.BestBid = msg
					}

					if server.InClients[msg.BidderId] == nil {
						fmt.Println("Bidder ID was not found! (at line 212)")
					}

					fmt.Println("Sending ack to channel now for bidder: ", msg.BidderId)

					server.InClients[msg.BidderId] <- &proto.Ack{
						Timestamp:         server.LamportClock.LocalEvent(),
						Response:          response,
						CurrentLeaderPort: server.Leader,
					}
					continue
				}
			case *proto.ResultMessage:

				if server.InClients[msg.CallerId] == nil {
					fmt.Println("Caller ID was not found! (at line 228)")
				}

				server.InClients[msg.CallerId] <- &proto.Outcome{
					Timestamp:         server.LamportClock.LocalEvent(),
					IsOver:            server.AuctionIsOver,
					Amount:            server.BestBid.Amount,
					CurrentLeaderPort: server.Leader,
				}
				continue

			default:
				//if we got to here something went wrong with the type of message being received in the queue
				fmt.Println("We got the wrong type of message!")
				fmt.Println("The request is of type: ", reflect.TypeOf(request))
				log.Panic("invalid message type")
			}

		}
	}()

	//stop from prematurely exiting main
	select {}

	//todo ensure to remove clients from the InClients map (all of them!)
	//todo ensure that the flag IsOver on an auction is activated!
	//TODO after an X amount of time, the leader must indicate the auction has stopped, broadcast to all clients the highest bid
	//time.Sleep(60 * time.Second)
	//timestamp := server.LamportClock.GetCurrentTimestamp()
	//server.Result(context.Background(), &timestamp)
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

		//Starter flag
		if match2 != nil {
			node.StartFlag <- true
			continue
		}

		port, _ := strconv.ParseInt(match[1], 10, 64)
		err = node.ConnectToNodeServer(port)
		if err != nil {
			//utility.LogAsJson(utility.LogStruct{Timestamp: node.GetTimestamp(), Identifier: node.PortId, Message: err.Error()}, true)
			continue
		}
	}
}

func (node *AuctionNode) ConnectToNodeServer(port int64) error {
	//Ensure no duplicates of servers
	if node.OutClients[port].Client != nil {
		return errors.New("Already connected to port: " + strconv.FormatInt(port, 10))
	}

	//Ensure server cannot make a client for itself
	if node.Port == port {
		return errors.New("Cannot connect to same port: " + strconv.FormatInt(port, 10))
	}

	node.LamportClock.LocalEvent()
	conn, err := grpc.NewClient("localhost:"+strconv.FormatInt(port, 10), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	client := proto.NewNodeClient(conn)

	//Ensure client cannot be nil
	if client == nil {
		return errors.New("Failed to create client for server: " + strconv.FormatInt(port, 10))
	}

	_, err = client.Ping(context.Background(), &proto.Empty{})
	if err != nil {
		return err
	}
	fmt.Println("Made connection to server: ", port)
	connection := connections.ClientConnection{Client: client, IsDown: false}
	node.OutClients[port] = connection
	//node.Replies[port] = make(chan bool, 1)

	return nil
}

// CallElection uses the bully algorithm to determine a leader.
// Based on the algorithm as described on https://en.wikipedia.org/wiki/Bully_algorithm last accessed 20/11/2025 TODO update date if necessary
func (node *AuctionNode) CallElection() {
	/*
		1. If P has the highest process ID, it sends a Victory message to all other processes and becomes the new Coordinator. Otherwise, P broadcasts an Election message to all other processes with higher process IDs than itself.
		2. If P receives no Answer after sending an Election message, then it broadcasts a Victory message to all other processes and becomes the Coordinator.
		3. If P receives an Answer from a process with a higher ID, it sends no further messages for this election and waits for a Victory message. (If there is no Victory message after a period of time, it restarts the process at the beginning.)
		4. If P receives an Election message from another process with a lower ID it sends an Answer message back and if it has not already started an election, it starts the election process at the beginning, by sending an Election message to higher-numbered processes.
		5. If P receives a Coordinator message, it treats the sender as the coordinator.
	*/

	fmt.Println("Election has started for ", node.NodeId)

	node.HasStartedAnElected = true
	IsHighest := true
	for connId, _ := range node.OutClients {
		if connId > node.NodeId {
			//it is not the highest
			fmt.Println("Found higher ID: ", connId)
			IsHighest = false
			break
		}
	}

	//if we are the highest ID then we need to broadcast out to everyone else that we are the leader.
	if IsHighest {
		for _, conn := range node.OutClients {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			//Ensure there exists a client for the connection
			//todo should we remove the client / connection here?
			client := conn.Client
			if client == nil {
				fmt.Println("Client not existing! (line 370)")
				continue
			}

			_, err := client.Coordinator(ctx, &proto.CoordinatorMessage{NodeId: node.NodeId})

			if err != nil {
				//TODO HANDLE CORRECTLY
				conn.IsDown = true
			}
		}
		node.Leader = node.NodeId
	} else {

		fmt.Println("I was not the highest!... I am now doing work.")
		higherNodesCount := 0 //count the number of nodes that are higher than it which it knows about
		failedNodesCount := 0 //count the number of those nodes that fail to respond
		wg := sync.WaitGroup{}
		for connId, conn := range node.OutClients {
			//only broadcast to every node which is higher than itself
			if connId < node.NodeId {
				continue
			}

			higherNodesCount++
			timestamp := node.LamportClock.LocalEvent()
			electionMsg := proto.ElectionMessage{Timestamp: timestamp, NodeId: node.NodeId}
			wg.Go(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()

				fmt.Println("Calling election with ", electionMsg.NodeId, " for node ", connId)

				client := conn.Client
				if client == nil {
					fmt.Println("Client not existing! (line 406)")
					return
				}

				_, err := client.Election(ctx, &electionMsg)
				if err != nil {

					if status.Code(err) == codes.Unavailable || strings.Contains(err.Error(), "EOF") {
						//Node is dead
						fmt.Println("The client doesn't exist!")
						delete(node.OutClients, connId)
						fmt.Println("Deleted connection ", connId)
					}

					fmt.Println("Got error (at line 420): ", err.Error())
					failedNodesCount++
				}
			})
		}
		wg.Wait()
		if failedNodesCount == higherNodesCount {
			node.AllFailed <- true
		}
		select {
		case <-node.ReceivedAnswer:
			select {
			case <-node.NewLeader:
				//A leader has been determined and therefore we can return
				break
			case <-time.After(5 * time.Second):
				//timeout reached, create new election
				defer node.CallElection()
				break
			}
			break
		case <-node.AllFailed:
			for connId, conn := range node.OutClients {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()

				client := conn.Client
				if client == nil {
					fmt.Println("Client not existing! (line 448)")
					continue
				}

				_, err := client.Coordinator(ctx, &proto.CoordinatorMessage{NodeId: node.NodeId})
				if err != nil {
					if status.Code(err) == codes.Unavailable || strings.Contains(err.Error(), "EOF") {
						//Node is dead
						fmt.Println("The client doesn't exist!")
						delete(node.OutClients, connId)
						fmt.Println("Deleted connection ", connId)
					}
				}
			}
			//Assign the node to know it became the leader
			node.Leader = node.NodeId
			break
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
		fmt.Println("The ID of received election is lower...")
		return nil, errors.New("the ID of received election cannot be lower that clients id")
	}

	//ID of the node we are getting the election from.
	clientId := electionMsg.GetNodeId()

	client := node.OutClients[clientId].Client
	if client == nil {
		return nil, errors.New("The client doesn't exist (at line 487)")
	}

	_, err := client.Answer(context.Background(), &proto.AnswerMessage{NodeId: node.NodeId, Timestamp: timestamp})
	if err != nil {
		if status.Code(err) == codes.Unavailable || strings.Contains(err.Error(), "EOF") {
			//Node is dead
			fmt.Println("The node is dead!")
			delete(node.OutClients, clientId)
			fmt.Println("Deleted client ", clientId)
		}
	}
	if node.HasStartedAnElected == false {
		fmt.Println("The id has not started an election... Starting it now")
		node.CallElection()
	}

	return nil, nil
}

// Coordinator is the logic the server must run when it receives a Coordinator rpc call from another server.
func (node *AuctionNode) Coordinator(ctx context.Context, msg *proto.CoordinatorMessage) (*proto.Empty, error) {
	//receiving a Coordinator call, means that the "bully"/leader has been decided and is the one who made the call
	node.Leader = msg.NodeId
	node.NewLeader <- true
	return nil, nil
}

// Answer is the logic the server must run when it receives an Answer rpc call from another server.
func (node *AuctionNode) Answer(ctx context.Context, answer *proto.AnswerMessage) (*proto.Empty, error) {
	if answer.NodeId <= node.NodeId {
		fmt.Println("The ID of received answer is lower...")
		return nil, errors.New("the ID of received answer cannot be lower that clients id")
	}

	//populate channel indicating an answer for use in the CallElection method
	fmt.Println("Got the answer! Sending it into channel now")
	node.ReceivedAnswer <- true
	fmt.Println("Returning empty proto now!")
	return &proto.Empty{}, nil
}

// Ping is the logic the server must run when it receives a Ping rpc call from another server.
func (node *AuctionNode) Ping(ctx context.Context, empty *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

// Bid is the logic the server must run when it receives a Bid rpc call from a client or server in the case it is the leader server
func (node *AuctionNode) Bid(ctx context.Context, bid *proto.BidMessage) (*proto.Ack, error) {

	if bid.GetWasForwarded() {
		fmt.Println("A replicate bid message has been recieved on node: ", node.NodeId)

		//Ensure the backup node also has the clients as channels!
		if node.InClients[bid.BidderId] == nil {
			node.InClients[bid.BidderId] = make(chan interface{}, 1)
		}

		//Double check to ensure the amount from the leader is actual a new highest bid.
		if node.BestBid.Amount > bid.GetAmount() {
			return &proto.Ack{
				Timestamp:         node.LamportClock.LocalEvent(),
				Response:          int32(utility.EXCEPTION),
				CurrentLeaderPort: node.Leader,
			}, nil
		}
		//Set the bid that arrived to be the best bid.
		node.BestBid = bid

		//Create acknowledgement to leader that it has arrived.

		fmt.Println("Sending ack now to leader: ", bid.BidderId)

		return &proto.Ack{
			Timestamp:         node.LamportClock.LocalEvent(),
			Response:          int32(utility.SUCCESS),
			CurrentLeaderPort: node.Leader,
		}, nil
	}

	fmt.Println("A bid has been received on node: ", node.NodeId)

	if node.InClients[bid.BidderId] == nil {
		//Create the channel for the new client
		fmt.Println("Made new client for bidder: ", bid.BidderId)
		node.InClients[bid.BidderId] = make(chan interface{}, 1)
	}

	node.LamportClock.RemoteEvent(bid.GetTimestamp())
	err := node.RequestQueue.Enqueue(bid)
	fmt.Println("The bid has been enqueued!")
	if err != nil {
		return nil, err
	}

	fmt.Println("Awaiting response")
	response := <-node.InClients[bid.BidderId]
	fmt.Println("Made it past the response")

	//Forward the state to other backup nodes
	ack, ok := response.(*proto.Ack)
	if !ok {
		fmt.Println("Internal server error")
		return nil, errors.New(fmt.Sprint("Internal Server Error"))
	}
	//Send out state updates to all backup nodes if leader
	if node.Leader == node.NodeId && ack.Response == int32(utility.SUCCESS) {
		fmt.Println("Starting replicate bid go routine")
		go func() {
			err2 := node.ReplicateBid(context.Background(), bid)
			if err2 != nil {
				fmt.Println("Replicate bid failed!")
			}
		}()
	}

	//Send response over to the client/leader
	fmt.Println("Returning ack now from bid!")
	return ack, nil
}

func (node *AuctionNode) ReplicateBid(ctx context.Context, bid *proto.BidMessage) error {

	if node.Leader != node.NodeId {
		return errors.New("this node is not the leader and cannot replicate bids")
	}
	//Make the bid have the flag WasForwarded turned on for backup nodes to handle.
	bid.WasForwarded = true

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	for connId, conn := range node.OutClients {

		client := conn.Client
		if client == nil {
			fmt.Println("Client not existing!")
			delete(node.OutClients, connId)
			fmt.Println("Deleted client ", connId)
			continue
		}

		ack, err := client.Bid(ctx, bid)
		if err != nil || ack.GetResponse() == int32(utility.EXCEPTION) || ack.GetResponse() == int32(utility.FAILURE) {
			log.Panicf("The backup server with port: %d did not respond with a SUCCESS", connId)
		}
	}
	return nil
}

func (node *AuctionNode) Result(ctx context.Context, msg *proto.ResultMessage) (*proto.Outcome, error) {
	node.LamportClock.RemoteEvent(msg.Timestamp)
	err := node.RequestQueue.Enqueue(msg)
	if err != nil {
		return nil, err
	}
	response := <-node.InClients[msg.CallerId]

	//prepare for returning the response
	return response.(*proto.Outcome), nil
}

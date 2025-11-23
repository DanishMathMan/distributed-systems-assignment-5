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
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"sync"
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

func CreateAuctionNode(port int64, id int64) AuctionNode {
	node := new(AuctionNode)
	node.Port = port
	node.NodeId = id
	node.LamportClock = lamportClock.CreateLamportClock()
	node.OutClients = make(map[int64]connections.ClientConnection)
	node.InClients = make(map[int64]chan interface{}, 1)
	node.Leader = id
	node.RequestQueue = utility.RequestQueue{}
	node.BestBid = &proto.BidMessage{Timestamp: 0, BidderId: -1, Amount: 0}
	node.Bids = make(map[int64]*proto.BidMessage)
	node.HasStartedAnElected = false
	node.AllFailed = make(chan bool, 1)
	node.ReceivedAnswer = make(chan bool, 1)
	node.NewLeader = make(chan bool, 1)
	node.StartFlag = make(chan bool)
	node.AuctionIsOver = false
	//node.BackupServers = make(map[int64]connections.ClientConnection)
	return *node
}

func main() {
	port := flag.Int64("port", 8080, "Input port for the server to start on. Note, port is also its id")
	flag.Parse()
	server := CreateAuctionNode(*port, *port)
	server.StartServer()

	go func() {
		err := server.InputHandler()
		if err != nil {
			//TODO HANDLE
		}
	}()

	//wait for starter flag to have been called
	<-server.StartFlag

	//continuously make sure the node has a connection to the leader
	go func() {
		for {
			//We are already leader, no need to ping
			if server.Leader == server.NodeId {
				time.Sleep(1 * time.Second)
				continue
			}

			time.Sleep(1 * time.Second)

			//Ping the current leader
			//even if the call times out and the leader is still alive (but slow), the election would just reelect the leader
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := server.OutClients[server.Leader].Client.Ping(ctx, nil)
			if err != nil {
				//The server did not respond!!! bad!
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

			//Check which type of request it is.
			v, ok := request.(proto.BidMessage)
			if ok == true {
				//Was the bid message forwarded?
				if v.GetWasForwarded() {

					//Double check to ensure the amount from the leader is actual a new highest bid.
					if server.BestBid.Amount > v.GetAmount() {
						server.InClients[v.BidderId] <- proto.Ack{
							Timestamp:         server.LamportClock.LocalEvent(),
							Response:          int32(utility.EXCEPTION),
							CurrentLeaderPort: server.Leader,
						}
					}
					//Set the bid that arrived to be the best bid.
					server.BestBid = &v

					//Create acknowledgement to leader that it has arrived.
					server.InClients[v.BidderId] <- proto.Ack{
						Timestamp:         server.LamportClock.LocalEvent(),
						Response:          int32(utility.SUCCESS),
						CurrentLeaderPort: server.Leader,
					}

				}

				//if the node is not leader, send an exception alongside information regarding which port is the leader
				if server.Leader != server.NodeId {
					server.InClients[v.BidderId] <- proto.Ack{
						Timestamp:         server.LamportClock.LocalEvent(),
						Response:          int32(utility.EXCEPTION),
						CurrentLeaderPort: server.Leader,
					}
					continue
				} else {
					//Node is the leader. Handle the bid.
					var response int32
					if v.Amount <= server.BestBid.Amount {
						response = int32(utility.FAILURE)
					} else {
						response = int32(utility.SUCCESS)
						server.BestBid = &v
					}

					server.InClients[v.BidderId] <- proto.Ack{
						Timestamp:         server.LamportClock.LocalEvent(),
						Response:          response,
						CurrentLeaderPort: server.Leader,
					}
					continue
				}
			} else {
				//If the request was an result message. (Can be handled by any node).
				v2, ok2 := request.(proto.ResultMessage)
				if ok2 == true {
					server.InClients[v2.CallerId] <- proto.Outcome{
						Timestamp:         server.LamportClock.LocalEvent(),
						IsOver:            server.AuctionIsOver,
						Amount:            server.BestBid.Amount,
						CurrentLeaderPort: server.Leader,
					}
					continue
				}
			}
			//if we got to here something went wrong with the type of message being received in the queue
			log.Panic("invalid message type")
		}
	}()

	//stop from prematurely exiting main
	select {}

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
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := c.Client.Coordinator(ctx, &proto.CoordinatorMessage{NodeId: node.NodeId})
			if err != nil {
				//TODO HANDLE CORRECTLY
				c.IsDown = true
			}
		}
		node.Leader = node.NodeId
	} else {
		higherNodesCount := 0 //count the number of nodes that are higher than it which it knows about
		failedNodesCount := 0 //count the number of those nodes that fail to respond
		wg := sync.WaitGroup{}
		for v, c := range node.OutClients {
			//only broadcast to every node which is higher than itself
			if v < node.NodeId {
				continue
			}
			higherNodesCount++
			timestamp := node.LamportClock.LocalEvent()
			electionMsg := proto.ElectionMessage{Timestamp: timestamp, NodeId: node.NodeId}
			wg.Go(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_, err := c.Client.Election(ctx, &electionMsg)
				if err != nil {
					//TODO HANDLE CORRECTLY
					c.IsDown = true
					failedNodesCount++
				}
			})
			// The node received an answer, therefore it must wait for receiving a Coordinator rpc call or timeout, whichever comes first
			//TODO HANDLE RECEIVING AN ANSWER PER STEP 3 AND 4
			//TODO WAIT FOR A VICTORY CALL OR TIMEOUT
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
			for _, c := range node.OutClients {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_, err := c.Client.Coordinator(ctx, &proto.CoordinatorMessage{NodeId: node.NodeId})
				if err != nil {
					//TODO HANDLE CORRECTLY
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
	node.NewLeader <- true
	return nil, nil
}

// Answer is the logic the server must run when it receives an Answer rpc call from another server.
func (node *AuctionNode) Answer(ctx context.Context, answer *proto.AnswerMessage) (*proto.Empty, error) {
	if answer.NodeId <= node.NodeId {
		//TODO THROW AN ERROR
	}

	//populate channel indicating an answer for use in the CallElection method
	node.ReceivedAnswer <- true
	return &proto.Empty{}, nil
}

// Ping is the logic the server must run when it receives a Ping rpc call from another server.
func (node *AuctionNode) Ping(ctx context.Context, empty *proto.Empty) (*proto.Empty, error) {
	return &proto.Empty{}, nil
}

// Bid is the logic the server must run when it receives a Bid rpc call from a client or server in the case it is the leader server
func (node *AuctionNode) Bid(ctx context.Context, bid *proto.BidMessage) (*proto.Ack, error) {
	node.LamportClock.RemoteEvent(bid.GetTimestamp())
	err := node.RequestQueue.Enqueue(bid)
	if err != nil {
		return nil, err
	}
	response := <-node.InClients[bid.GetBidderId()]

	//Forward the state to other backup nodes

	//Send out state updates to all backup nodes if leader
	if node.Leader != node.NodeId {
		//Make the bid have the flag WasForwarded turned on for backup nodes to handle.
		bid.WasForwarded = true
		for k, v := range node.OutClients {
			ack, err := v.Client.Bid(nil, bid)
			if err != nil || ack.GetResponse() == int32(utility.EXCEPTION) || ack.GetResponse() == int32(utility.FAILURE) {
				log.Panicf("The backup server with port: %d did not respond with a SUCCESS", k)
			}
		}
	}

	//Send response over to the client/leader
	return response.(*proto.Ack), nil
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

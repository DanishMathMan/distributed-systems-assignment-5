package main

import (
	"bufio"
	proto "distributed-systems-assignment-5/src/grpc"
	"distributed-systems-assignment-5/src/utility"
	"distributed-systems-assignment-5/src/utility/connections"
	"distributed-systems-assignment-5/src/utility/lamportClock"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

func main() {
	primaryPort := flag.Int64("primary-port", -1, "Input port for the client to associate with the primary server. Note, port is also its id")
	backupPort := flag.Int64("backup-port", -1, "Input port for the client to associate with a backup server. Note, port is also its id")
	flag.Parse()

	if *primaryPort == -1 {
		err := errors.New("missing primary port. Use --primary-port to specify the primary port of the leader node")
		fmt.Println(err)
		os.Exit(1)
	}

	if *backupPort == -1 {
		err := errors.New("missing backup port. Use --backup-port to specify the backup port of a non-leader node")
		fmt.Println(err)
		os.Exit(1)
	}

	client := CreateAuctionClient(*primaryPort)
	client.ConnectClientToNode(*primaryPort)
	client.ConnectClientToNode(*backupPort)
	go func() {
		err := client.InputHandler()
		if err != nil {
			return
		}
	}()

	//prevents program from terminating prematurely
	select {}
}

func (client *AuctionClient) ConnectClientToNode(port int64) {
	connection, err := grpc.NewClient("localhost:"+strconv.FormatInt(port, 10), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	c := proto.NewNodeClient(connection)
	client.ServerPool[port] = connections.ClientConnection{Client: c}
}

func (client *AuctionClient) InputHandler() error {

	BidRegex := regexp.MustCompile("--bid *(?P<bid>\\d{0,7})")
	ResultRegex := regexp.MustCompile("--request")

	for {
		reader := bufio.NewReader(os.Stdin)
		msg, err := reader.ReadString('\n')
		if err != nil {
			continue
		}

		BidMatch := BidRegex.FindStringSubmatch(msg)
		ResultMatch := ResultRegex.FindStringSubmatch(msg)

		if BidMatch != nil {
			err := client.OnBidRequestResponse(BidMatch)
			if err != nil {
				return err
			}
		}

		if ResultMatch != nil {
			err := client.OnResultRequestResponse()
			if err != nil {
				return err
			}
		}
	}
}

//TODO (!==!) HANDLE LEADER UPDATE (!==!)

func (client *AuctionClient) OnBidRequestResponse(BidMatch []string) error {
	//do bidding
	// todo bid must be higher than the current highest bid
	timestamp := client.LamportClock.LocalEvent()
	bidAmount, _ := strconv.ParseInt(BidMatch[1], 10, 64)
	bid := &proto.BidMessage{Timestamp: timestamp, BidderId: client.ClientId, Amount: bidAmount}
	ack, err := client.CurrentLeader.Client.Bid(nil, bid)
	if err != nil {
		return err //TODO is this where we will check if the server is down and then try to connect to another?
	}
	//todo handle ack
	timestamp = client.LamportClock.RemoteEvent(ack.Timestamp)
	switch ack.Response {
	case int32(utility.EXCEPTION):
		//Todo handle
		break //TODO can be handled in another way
	case int32(utility.SUCCESS):
		//TODO handle
		break
	case int32(utility.FAILURE):
		//TODO HANDLE
		break
	}
	return nil
}

func (client *AuctionClient) OnResultRequestResponse() error {
	//do result getting
	client.LamportClock.LocalEvent()
	result, err := client.CurrentLeader.Client.Result(nil, nil)
	if err != nil {
		return err //TODO is this where we will check if the server is down and then try to connect to another?
	}
	//handle result
	client.LamportClock.RemoteEvent(result.Timestamp)
	if result.IsOver {
		fmt.Println("[AUCTION] IS OVER!")
		fmt.Println(fmt.Sprintf("Winning Highest Bid: %d", result.Amount))
	} else {
		fmt.Println("[AUCTION] IS STILL GOING!")
		fmt.Println(fmt.Sprintf("Current Highest Bid: %d", result.Amount))
	}
	return nil
}

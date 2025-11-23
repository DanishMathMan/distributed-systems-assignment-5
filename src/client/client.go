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
	"os"
	"regexp"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type AuctionClient struct {
	ClientId      int64
	PrimaryPort   int64
	BackupPort    int64
	ServerPool    map[int64]connections.ClientConnection
	CurrentLeader connections.ClientConnection
	LamportClock  lamportClock.LamportClock
}

func CreateAuctionClient(id int64, primaryPort int64, backupPort int64) AuctionClient {
	client := new(AuctionClient)
	client.ClientId = id
	client.PrimaryPort = primaryPort
	client.BackupPort = backupPort
	client.LamportClock = lamportClock.CreateLamportClock()
	client.ServerPool = make(map[int64]connections.ClientConnection)
	client.CurrentLeader = client.ServerPool[client.PrimaryPort]
	return *client
}

func main() {
	//TODO Consider using proper GUID / UUID instead of user input?
	id := flag.Int64("id", -1, "ID of Client. MUST BE UNIQUE (!)")
	primaryPort := flag.Int64("primary-port", -1, "Input port for the client to associate with the primary server. Note, port is also its id")
	backupPort := flag.Int64("backup-port", -1, "Input port for the client to associate with a backup server. Note, port is also its id")
	flag.Parse()

	if *id == -1 {
		err := errors.New("missing ID of client. Use --client to specify ID of the client. Do NOT use the same ID as another client you've created")
		fmt.Println(err)
		os.Exit(1)
	}

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

	client := CreateAuctionClient(*id, *primaryPort, *backupPort)
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
	client.CurrentLeader = client.ServerPool[client.PrimaryPort] // TODO need to correctly connect to leader node
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ack, err := client.CurrentLeader.Client.Bid(ctx, bid)
	if err != nil {

		//Assume its the timeout error we've gotten. Send up a request to the backup so we cna get an acknowledgement
		defer cancel()

		//Reassign the acknowledgement to be from the backup to update our client to the new leader.
		ack, err = client.ServerPool[client.BackupPort].Client.Bid(ctx, bid)

	}
	timestamp = client.LamportClock.RemoteEvent(ack.Timestamp)

	//Check if our clients primary port (aka the leaders port) is different from the one from ack.
	if ack.GetCurrentLeaderPort() != client.PrimaryPort {
		//Update the clients leader port
		client.PrimaryPort = ack.GetCurrentLeaderPort()
		client.ConnectClientToNode(ack.GetCurrentLeaderPort())
	}

	switch ack.Response {
	case int32(utility.EXCEPTION):
		fmt.Println(fmt.Sprintf("You are without leader! SHAME!"))
		break
	case int32(utility.SUCCESS):
		fmt.Println(fmt.Sprintf("You've succesfully bidded %d", bid.Amount))
		break
	case int32(utility.FAILURE):
		fmt.Println(fmt.Sprintf("Your bid was lower than the current highest bid"))
		break
	}
	return nil
}

func (client *AuctionClient) OnResultRequestResponse() error {
	//do result getting
	timestamp := proto.ResultMessage{Timestamp: client.LamportClock.LocalEvent()}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := client.CurrentLeader.Client.Result(ctx, &timestamp)
	if err != nil {
		//Assume its the timeout error we've gotten. Send up a request to the backup so we cna get an acknowledgement

		//Reassign the acknowledgement to be from the backup to update our client to the new leader.
		result, err = client.ServerPool[client.BackupPort].Client.Result(ctx, &timestamp)
	}
	//handle result
	client.LamportClock.RemoteEvent(result.Timestamp)

	//Check if our clients primary port (aka the leaders port) is different from the one from ack.
	if result.GetCurrentLeaderPort() != client.PrimaryPort {

		//Update the clients leader port
		client.PrimaryPort = result.GetCurrentLeaderPort()
		client.ConnectClientToNode(result.GetCurrentLeaderPort())
	}

	if result.IsOver {
		fmt.Println("[AUCTION] IS OVER!")
		fmt.Println(fmt.Sprintf("Winning Highest Bid: %d", result.Amount))
	} else {
		fmt.Println("[AUCTION] IS STILL GOING!")
		fmt.Println(fmt.Sprintf("Current Highest Bid: %d", result.Amount))
	}
	return nil
}

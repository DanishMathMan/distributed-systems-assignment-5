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
	client.CurrentLeader = connections.ClientConnection{}
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

	//goroutine to always wait for the auction to end and report the result
	go client.AuctionEnd()

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

	//Add connection to map of nodes.
	client.ServerPool[port] = connections.ClientConnection{Client: c}

	//Update leader node if it's not been set.
	if client.CurrentLeader != client.ServerPool[client.PrimaryPort] {
		client.CurrentLeader = client.ServerPool[client.PrimaryPort]
	}

}

func (client *AuctionClient) InputHandler() error {

	BidRegex := regexp.MustCompile("--bid *(?P<bid>\\d{0,7})")
	ResultRegex := regexp.MustCompile("--result")

	fmt.Println("-- Welcome to our auction --")
	fmt.Println("To bid please type: --bid (amount)")
	fmt.Println("To see the current result of auction type: --result")

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

func (client *AuctionClient) OnBidRequestResponse(BidMatch []string) error {
	timestamp := client.LamportClock.LocalEvent()
	bidAmount, _ := strconv.ParseInt(BidMatch[1], 10, 64)
	bid := &proto.BidMessage{Timestamp: timestamp, BidderId: client.ClientId, Amount: bidAmount, WasForwarded: false}

	leaderClient := client.ServerPool[client.PrimaryPort].Client
	if leaderClient == nil {
		fmt.Println("No leader client found")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ack, err := leaderClient.Bid(ctx, bid)
	if err != nil {
		//Assume its the timeout error we've gotten. Send up a request to the backup so we cna get an acknowledgement
		fmt.Println("Gave up on leader node. Trying backup!")

		backupClient := client.ServerPool[client.BackupPort].Client
		if backupClient == nil {
			fmt.Println("No backup client found")
		}

		ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel2()

		//Reassign the acknowledgement to be from the backup to update our client to the new leader.
		ack, err = client.ServerPool[client.BackupPort].Client.Bid(ctx2, bid)

		if err != nil {
			fmt.Println("Gave up on backup node. Ggs!")
			fmt.Println("No extra clients to use. Goodbye")
			os.Exit(1)
		}

	}

	if ack == nil {
		fmt.Println("Ack was nil!")
		return nil
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
		fmt.Println(fmt.Sprintf("Something went wrong. Please bid again."))
		break
	case int32(utility.SUCCESS):
		fmt.Println(fmt.Sprintf("You've succesfully bidded %d", bid.Amount))
		break
	case int32(utility.FAILURE):
		fmt.Println(fmt.Sprintf("Your bid was lower than the current highest bid"))
		break
	case int32(utility.ISOVER):
		fmt.Println(fmt.Sprintf("The auction is over. To see who won type the command: '--result'"))
	}
	return nil
}

// waits for RPC AuctionEnd() from the server
func (client *AuctionClient) AuctionEnd() {
	for {
		// needs to find who leader is such that we can call the method on leader node
		leaderClient := client.ServerPool[client.PrimaryPort].Client
		if leaderClient == nil {
			fmt.Println("No leader client found")
			continue
		}

		// 30 seconds determines how long the client will wait for the WaitForAuctionEnd response from the leader node
		// Note: if you change auctionTimeout then you have to change this value accordingly, maybe use a config file for the auctionTimeout value so both can share?
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		outcome, err := leaderClient.AuctionEnd(ctx, &proto.Empty{})
		cancel() // cancel stops context timer

		// If leader is unresponsive, like it died then dont panic, just wait a bit and try a backup instead
		if err != nil {
			fmt.Println("Waiting for auction end failed on leader, trying backup:", err)

			backupClient := client.ServerPool[client.BackupPort].Client
			if backupClient == nil {
				fmt.Println("No backup client found")
				continue
			}

			// 30 seconds determines how long the client will wait for the AuctionEnd response from the backup node
			ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
			outcome, err = backupClient.AuctionEnd(ctx2, &proto.Empty{})
			cancel2()

			if err != nil {
				fmt.Println("Waiting for auction end, failed on backup node", err)
				continue
			}
		}

		if outcome == nil {
			fmt.Println("Outcome was nil!")
			continue
		}

		client.handleOutcome(outcome)
		return
	}
}

// moved some print code from OnResultRequestResponse() into a helper method to make it reusable
func (client *AuctionClient) handleOutcome(result *proto.Outcome) {
	if result.IsOver {
		fmt.Println("[AUCTION] IS OVER!")
		fmt.Println(fmt.Sprintf("Winning Highest Bid: %d", result.Amount))
	} else {
		fmt.Println("[AUCTION] IS STILL GOING!")
		fmt.Println(fmt.Sprintf("Current Highest Bid: %d", result.Amount))
	}
}

func (client *AuctionClient) OnResultRequestResponse() error {

	timestamp := proto.ResultMessage{Timestamp: client.LamportClock.LocalEvent(), CallerId: client.ClientId}

	leaderClient := client.ServerPool[client.PrimaryPort].Client
	if leaderClient == nil {
		fmt.Println("No leader client found")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := leaderClient.Result(ctx, &timestamp)
	if err != nil {
		//Create new timeout context to send up to backup

		backupClient := client.ServerPool[client.BackupPort].Client
		if backupClient == nil {
			fmt.Println("No backup client found")
		}

		ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel2()

		//Reassign the acknowledgement to be from the backup to update our client to the new leader.
		result, err = backupClient.Result(ctx2, &timestamp)

		if err != nil {
			fmt.Println("No extra clients to use. Goodbye")
			os.Exit(1)
		}
	}

	if result == nil {
		fmt.Println("Result was nil!")
		return nil
	}
	client.LamportClock.RemoteEvent(result.Timestamp)

	if result.GetCurrentLeaderPort() != client.PrimaryPort {
		client.PrimaryPort = result.GetCurrentLeaderPort()
		client.ConnectClientToNode(result.GetCurrentLeaderPort())
	}
	client.handleOutcome(result)
	return nil
}

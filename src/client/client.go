package client

import (
	"bufio"
	proto "distributed-systems-assignment-5/src/grpc"
	"distributed-systems-assignment-5/src/utility"
	"distributed-systems-assignment-5/src/utility/connections"
	"errors"
	"flag"
	"log"
	"os"
	"regexp"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	primaryPort := flag.Int64("primary-port", 8080, "Input port for the client to associate with the primary server. Note, port is also its id")
	backupPort := flag.Int64("backup-port", 8081, "Input port for the client to associate with a backup server. Note, port is also its id")
	flag.Parse()

	if primaryPort == nil || backupPort == nil {
		//TODO throw error and ensure primary + backup port is entered.
		_ = errors.New("you done goofed.")
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
	connection, err := grpc.NewClient("localhost"+strconv.FormatInt(port, 10), grpc.WithTransportCredentials(insecure.NewCredentials()))
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
			//todo consider making a separate function for code below
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
				break
			case int32(utility.SUCCESS):
				//TODO handle
				break
			case int32(utility.FAILURE):
				//TODO HANDLE
				break
			}
		}

		if ResultMatch != nil {
			//TODO consider making a separate function
			//do result getting
			client.LamportClock.LocalEvent()
			result, err := client.CurrentLeader.Client.Result(nil, nil)
			if err != nil {
				return err //TODO is this where we will check if the server is down and then try to connect to another?
			}
			//handle result
			client.LamportClock.RemoteEvent(result.Timestamp)
			if result.IsOver {
				//todo Handle logic for when an auction is over
			}
			//TODO handle logic for when the auction is NOT over
		}
	}
}

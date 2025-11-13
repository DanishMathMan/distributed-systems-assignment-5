package client

import (
	"bufio"
	proto "distributed-systems-assignment-5/src/grpc"
	"distributed-systems-assignment-5/src/utility/connections"
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

func main(){
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
			//do bidding
			timestamp := client.LamportClock.LocalEvent()
			bid := proto.BidMessage{Timestamp: timestamp, BidderId: client.ClientId, Amount: }
		}

		if ResultMatch != nil {
			//do result getting
			timestamp := client.LamportClock.LocalEvent()
		}

		fmt.Println("We've got a bid!")

	}
}

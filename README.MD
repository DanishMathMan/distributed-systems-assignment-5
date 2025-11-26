# Distributed Systems, BSc (Autumn 2025) - Assignment 4
## by freju@itu.dk, frvs@itu.dk and alyp@itu.dk


### Prerequisites:
- Golang (1.25+)
- Basic knowledge about running go programs.
- google.golang.org/grpc v1.76.0 compatible installation
- google.golang.org/protobuf v1.36.10 compatible installation

## How to run:

## Node usage:
1. Navigate to the directory where you've downloaded the github repository\
   ```cd <directoryOfRepository>/node```
2. Run the node specifying the port it will listen on and the port of the leader (the leader server doesn't need to be started yet): \
   ```go run node.go --port <port> --leader <leaders port>``` \
   e.x: ```go run node.go --port 8080 --leader 8081```
3. Repeat step 1 and 2 in a new terminal at least twice (so you have a backup server and a leader server)
4. Ensure a node is started on the leaders port in the beginning. After wards nodes will handle elections and replication.
4. Connect to other nodes' servers by writing in the command line. **Note** only a 4 digit port is accepted:\
   ``` --connect <port>``` or ```-c <port>```
5. Repeat step 4 for each port to connect to for each node terminal opened.
6. Once you've connected and made the servers you want, run ```--start``` in each node terminal.

## Client Start: 
1. Navigate to the directory where you've downloaded the github repository\
   ```cd <directoryOfRepository>/client```
2. Run the client specifying an id, the port of the leader server and the port of any backup server. \
   ```go run client.go --id <number> --primary-port <leaders port> --backup-port <backups port>``` \
   e.x: ```go run client.go --id 1 --primary-port 8081 --backup-port 8080```

## Client Usage:
1. When a client is running you can either bid on an auction with: \
```--bid <amount>``` or request the current state of the auction with: ```--result```

### Minimal example:
This example is to show how the program runs. It will handle a leader crash and can run as long as there's other active nodes to take over the leader node.
1. Terminal 1: ```go run node.go --port 8080 --leader 8081```
2. Terminal 2: ```go run node.go --port 8080 --leader 8081```
4. Terminal 1: ```-c 8081```
5. Terminal 2: ```-c 8080```
7. Terminal 1: ```--start```
8. Terminal 2: ```--start```
9. Terminal 3: ```go run client.go --id 1 --primary-port 8081 --backup-port 8080```
10. Terminal 3 ```--bid 10```
11. Terminal 3 ```--result```
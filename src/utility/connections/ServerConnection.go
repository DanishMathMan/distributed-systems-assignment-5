package connections

import proto "distributed-systems-assignment-5/src/grpc"

type ServerConnection struct {
	Server proto.NodeServer
	IsDown bool
}

package connections

import proto "distributed-systems-assignment-5/src/grpc"

type ClientConnection struct {
	Client proto.NodeClient
	IsDown bool
}

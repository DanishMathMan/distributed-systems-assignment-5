package utility

import proto "distributed-systems-assignment-5/src/grpc"

// queue Reimplemented from previous assignment: https://github.com/DanishMathMan/Distributed-Systems-Assignment-04/blob/main/src/utility/queue.go

type BidQueue struct {
	data []*proto.BidMessage
}

// func for enqueuing into the queue
func (queue *BidQueue) Enqueue(in *proto.BidMessage) {
	queue.data = append(queue.data, in)
}

// func for dequeuing into the queue
func (queue *BidQueue) Dequeue() *proto.BidMessage {
	dequeued := queue.data[0]
	queue.data = queue.data[1:]
	return dequeued
}

func (queue *BidQueue) IsEmpty() bool {
	return len(queue.data) == 0
}

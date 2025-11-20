package utility

import proto "distributed-systems-assignment-5/src/grpc"

// queue Reimplemented from previous assignment: https://github.com/DanishMathMan/Distributed-Systems-Assignment-04/blob/main/src/utility/queue.go

type BidQueue struct {
	data []*proto.BidMessage
}

// Enqueue inserts an element into the queue
func (queue *BidQueue) Enqueue(in *proto.BidMessage) {
	queue.data = append(queue.data, in)
}

// Dequeue removes and returns the next element of the queue
func (queue *BidQueue) Dequeue() *proto.BidMessage {
	dequeued := queue.data[0]
	queue.data = queue.data[1:]
	return dequeued
}

// IsEmpty returns true if there are no elements in the queue and false otherwise.
func (queue *BidQueue) IsEmpty() bool {
	return len(queue.data) == 0
}

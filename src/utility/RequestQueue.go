package utility

import (
	proto "distributed-systems-assignment-5/src/grpc"
	"errors"
	"reflect"
)

// queue Reimplemented from previous assignment: https://github.com/DanishMathMan/Distributed-Systems-Assignment-04/blob/main/src/utility/queue.go

type RequestQueue struct {
	data []interface{}
}

// Enqueue inserts an element into the queue
func (queue *RequestQueue) Enqueue(in interface{}) error {
	if reflect.TypeOf(in) != reflect.TypeOf(proto.BidMessage{}) || reflect.TypeOf(in) != reflect.TypeOf(proto.Ack{}) {
		return errors.New("the request must be of type *proto.BidMessage or *proto.Ack")
	}
	queue.data = append(queue.data, in)
	return nil
}

// Dequeue removes and returns the next element of the queue
func (queue *RequestQueue) Dequeue() interface{} {
	dequeued := queue.data[0]
	queue.data = queue.data[1:]
	return dequeued
}

// IsEmpty returns true if there are no elements in the queue and false otherwise.
func (queue *RequestQueue) IsEmpty() bool {
	return len(queue.data) == 0
}

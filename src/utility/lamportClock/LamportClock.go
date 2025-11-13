package lamportClock

type LamportClock struct {
	t chan int64
}

//goland:noinspection GoExportedFuncWithUnexportedType
func CreateLamportClock() LamportClock {
	return LamportClock{make(chan int64, 1)}
}

// LocalEvent updates the Timestamp by incrementing it by 1
func (l *LamportClock) LocalEvent() int64 {
	newTimestamp := <-l.t + 1
	l.t <- newTimestamp
	return newTimestamp
}

// RemoteEvent updates the Timestamp by setting it to one greater than the maximum of the current Timestamp and the parameter provided timestamp
func (l *LamportClock) RemoteEvent(otherTimestamp int64) int64 {
	newTimestamp := max(<-l.t, otherTimestamp) + 1
	l.t <- newTimestamp
	return newTimestamp
}

func (l *LamportClock) GetCurrentTimestamp() int64 {
	t := <-l.t
	l.t <- t
	return t
}

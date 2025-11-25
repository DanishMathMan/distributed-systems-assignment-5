package utility

type ResponseEnum int32

const (
	EXCEPTION ResponseEnum = iota
	SUCCESS
	FAILURE
	ISOVER
)

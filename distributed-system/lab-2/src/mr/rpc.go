package mr

//
// RPC definitions.
//
// remember to capitalize all names.
//

import (
	"os"
	"strconv"
)

type GetTaskArgs struct {
}

type TaskType string // represent tasktype, either map or reduce task

type GetTaskReply struct {
	Name            string // the name of file that acts as the input
	Number          int    // task number (to be refferred each phases, not to be confused with partition ID to put to reduce)
	PartitionNumber int    // partition number, to match with which reducer will take it
	Type            TaskType
}

type UpdateTaskStatusArgs struct {
	Name string
	Type TaskType
}

type UpdateTaskStatusReply struct {
}

var (
	mapType    TaskType = "map"
	reduceType TaskType = "reduce"
)

// Add your RPC definitions here.

// Cook up a unique-ish UNIX-domain socket name
// in /var/tmp, for the coordinator.
// Can't use the current directory since
// Athena AFS doesn't support UNIX-domain sockets.
func coordinatorSock() string {
	s := "/var/tmp/5840-mr-"
	s += strconv.Itoa(os.Getuid())
	return s
}

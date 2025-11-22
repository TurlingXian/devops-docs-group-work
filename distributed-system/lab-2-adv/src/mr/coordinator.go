package mr

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"sync"
	"time"
)

const timeOutCoefficient = 10
const defaultWorkerPort = 30022

var (
	unstarted  Status = "unstarted"
	inprogress Status = "inprogress"
	completed  Status = "completed"
)

type Coordinator struct {
	// Your definitions here.
	mapTasks         map[string]*TaskMetadata // a map of map task
	reduceTasks      map[string]*TaskMetadata //a map of reduce task
	cond             *sync.Cond               //condition variable (mutex)
	mapRemaining     int
	reduceRemaining  int
	numbeOfReduce    int      // number of "reduce" workes, used in pair with partition key
	mapTaskAddresses []string // addressbook of all map workers
}

// Your code here -- RPC handlers for the worker to call.

// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.

type Status string // indicate the status, done, undone,...

type TaskMetadata struct {
	number    int // the sequence number to mark a task, i.e: taks 1-1, task 1-2 (task 1, partition 1, task 1 partition 2)
	startTime time.Time
	status    Status
}

// get task
func (c *Coordinator) GetMapTask() (string, int) {
	for task := range c.mapTasks {
		if c.mapTasks[task].status == unstarted {
			c.mapTasks[task].startTime = time.Now().UTC()
			c.mapTasks[task].status = inprogress
			return task, c.mapTasks[task].number
		}
	}
	return "", -1
}

// get reduce task
func (c *Coordinator) GetReduceTask() (string, int) {
	for task := range c.reduceTasks {
		if c.reduceTasks[task].status == unstarted {
			c.reduceTasks[task].startTime = time.Now().UTC()
			c.reduceTasks[task].status = inprogress
			return task, c.reduceTasks[task].number
		}
	}
	return "", -1
}

// get task reply (for worker called to coordinator)
func (c *Coordinator) GetTask(args *GetTaskArgs, reply *GetTaskReply) error {
	c.cond.L.Lock() // lock the conditional variable when accessing shared variable
	// check map task
	if c.mapRemaining > 0 {
		// still have remaining
		// check the queue
		mapTask, indexOfTask := c.GetMapTask()
		for mapTask == "" {
			if c.mapRemaining == 0 {
				break
			}
			c.cond.Wait()
			mapTask, indexOfTask = c.GetMapTask()
		}
		if mapTask != "" {
			reply.Name = mapTask
			reply.Number = indexOfTask
			reply.Type = mapType
			reply.PartitionNumber = c.numbeOfReduce
			c.cond.L.Unlock()
			return nil
		}
		reply.Type = waitType
		c.cond.L.Unlock()
		return nil
	}
	// instead of directly goes to reduce, set the map worker in wait state

	// check reduce task
	if c.reduceRemaining > 0 {
		reduceTask, numberOfReduceTask := c.GetReduceTask()
		for reduceTask == "" {
			if c.reduceRemaining == 0 {
				c.cond.L.Unlock()
				return errors.New("all tasks are completed, no more remaining")
			}
			c.cond.Wait()
			reduceTask, numberOfReduceTask = c.GetReduceTask()
		}
		if reduceTask != "" {
			reply.Name = reduceTask
			reply.Number = numberOfReduceTask
			reply.Type = reduceType
			reply.MapAddresses = c.mapTaskAddresses
			c.cond.L.Unlock()
			return nil
		}
		// dont have to fetch the queue because reduce task can be taken from output of map
		reply.Type = waitType
		c.cond.L.Unlock()
		return nil
	}

	c.cond.L.Unlock()
	return errors.New("all tasks are completed, no more remaining")
}

// a function to handle the failed worker
func (c *Coordinator) ReportMapWorkerFailure(args *ReportFailureArgs, reply *ReportFailureReply) error {
	c.cond.L.Lock() // lock when accessing shared data
	defer c.cond.L.Unlock()

	var taskName string
	for name, metadata := range c.mapTasks {
		if metadata.number == args.MapTaskIndex { // found a task that was reported as failed
			taskName = name // end soon
			break
		}
	}

	if taskName != "" && c.mapTasks[taskName].status == completed {
		log.Printf("Reported map task %d failed, reschedule...", args.MapTaskIndex)
		c.mapTasks[taskName].status = unstarted
		c.mapRemaining++
	}

	return nil
}

// update the status for map or reduce task
func (c *Coordinator) UpdateTaskStatus(args *UpdateTaskStatusArgs, reply *UpdateTaskStatusReply) error {
	c.cond.L.Lock()
	defer c.cond.L.Unlock()

	if args.Type == mapType { // must collect the information of the map worker first
		taskMetadata, exists := c.mapTasks[args.Name]
		if !exists {
			log.Printf("CRITICAL WARNING: Worker tried to update unknown Map Task: %s", args.Name)
			return nil
		}

		if taskMetadata.status != completed {
			taskMetadata.status = completed
			c.mapRemaining -= 1

			currentMapWorkerID := taskMetadata.number

			if currentMapWorkerID >= 0 && currentMapWorkerID < len(c.mapTaskAddresses) {
				c.mapTaskAddresses[currentMapWorkerID] = args.WorkerAddress
			} else {
				log.Printf("CRITICAL: Map Worker ID %d out of bounds", currentMapWorkerID)
			}
		}
	} else {
		taskMetadata, exists := c.reduceTasks[args.Name]
		if !exists {
			log.Printf("CRITICAL WARNING: Worker tried to update unknown Reduce Task: %s", args.Name)
			return nil
		}

		if taskMetadata.status != completed {
			taskMetadata.status = completed
			c.reduceRemaining -= 1
		}
	}
	return nil
}

// if a task is dead, a worker is down, something needs to be rearranged, we call this
func (c *Coordinator) Rescheduler() {
	for {
		time.Sleep(100 * time.Millisecond) // avoid the "spinning" logic ~ busy wait
		c.cond.L.Lock()                    // if we want to change, better accquired the lock first
		if c.mapRemaining != 0 {
			for task := range c.mapTasks {
				currentTime := time.Now().UTC()
				startTime := c.mapTasks[task].startTime
				status := c.mapTasks[task].status
				if status == inprogress {
					different := currentTime.Sub(startTime)
					if different > timeOutCoefficient*time.Second {
						// if the task was running too long, assume that we have 10
						// log.Printf("Rescheduling a task with name '%s', type of this task '%s'.", task, mapType)
						c.mapTasks[task].status = unstarted
						c.cond.Broadcast() // signal the GetTask function, the Wait() function
					}
				}
			}
		} else if c.reduceRemaining != 0 {
			c.cond.Broadcast() // signal the fetch (GetTask) to get reduce task or wait,...
			for task := range c.reduceTasks {
				currentTime := time.Now().UTC()
				startTime := c.reduceTasks[task].startTime
				status := c.reduceTasks[task].status
				if status == inprogress {
					different := currentTime.Sub(startTime)
					if different > timeOutCoefficient*time.Second { // double check for the time, if not specified the second -> take nanosecond -> extremely fast -> create redundant tasks
						// log.Printf("Rescheduling a task with name '%s', type of this task '%s'.", task, reduceType)
						c.reduceTasks[task].status = unstarted
						c.cond.Broadcast()
					}
				}
			}
		} else {
			c.cond.Broadcast() // if no task remains, then broadcast too
			c.cond.L.Unlock()  // end the accessing shared database
			break
		}
		c.cond.L.Unlock() // simply unlock
	}
}

// function to just signal the "main" program that we have done

// main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
func (c *Coordinator) Done() bool {
	ret := false
	c.cond.L.Lock()
	defer c.cond.L.Unlock()
	if c.reduceRemaining == 0 {
		return true
	}
	return ret
}

// start a thread that listens for RPCs from worker.go
func (c *Coordinator) server() {
	rpc.Register(c)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := coordinatorSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

// create a Coordinator.
// main/mrcoordinator.go calls this function.
// nReduce is the number of reduce tasks to use.
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	mapTask := map[string]*TaskMetadata{}
	for i, file := range files {
		mapTask[file] = &TaskMetadata{
			number: i,
			status: unstarted,
		}
	}

	reduceTask := map[string]*TaskMetadata{}
	for i := 0; i < nReduce; i++ {
		reduceTask[fmt.Sprintf("%d", i)] = &TaskMetadata{
			number: i,
			status: unstarted,
		}
	}

	mu := sync.Mutex{}

	cond := sync.NewCond(&mu)

	c := Coordinator{
		mapTasks:         mapTask,
		reduceTasks:      reduceTask,
		mapRemaining:     len(files),
		reduceRemaining:  nReduce,
		numbeOfReduce:    nReduce,
		cond:             cond,
		mapTaskAddresses: make([]string, len(files)), // space equals to length of the passed files
	}

	// Your code here.

	// start a new rountine for the rescheduler (constanly check for exceeded time task independently)
	go c.Rescheduler()

	c.server()
	return &c
}

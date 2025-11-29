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

var (
	unstarted  Status = "unstarted"
	inprogress Status = "inprogress"
	completed  Status = "completed"
)

type Coordinator struct {
	mapTasks        map[string]*TaskMetadata // a map of map task
	reduceTasks     map[string]*TaskMetadata //a map of reduce task
	cond            *sync.Cond               //condition variable (mutex)
	mapRemaining    int
	reduceRemaining int
	numbeOfReduce   int // number of "reduce" workes, used in pair with partition key
	workerMap       map[string]*WorkerInfor
}

type Status string // indicate the status, done, undone,...

type TaskMetadata struct {
	number         int // the sequence number to mark a task, i.e: taks 1-1, task 1-2 (task 1, partition 1, task 1 partition 2)
	startTime      time.Time
	status         Status
	assignedWorker string
}

type WorkerInfor struct {
	WorkerAddress    string
	LastReportedTime time.Time
	WorkerDown       bool
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
	// in the get task, include a map of dictonary book
	locations := make([]string, len(c.mapTasks))
	for name, taskMetaData := range c.mapTasks {
		if name != "" && taskMetaData.assignedWorker != "" {
			locations[taskMetaData.number] = taskMetaData.assignedWorker
		}
	}

	if c.mapRemaining != 0 {
		// check if we still have map task in the queue
		mapTask, numberOfMapTask := c.GetMapTask()
		for mapTask == "" { // the queue is empty
			if c.mapRemaining == 0 { // no job
				break
			}
			// still have job
			c.cond.Wait() // wait for accquiring the lock
			mapTask, numberOfMapTask = c.GetMapTask()
		}
		if mapTask != "" { // have somthing in the queue
			reply.Name = mapTask                                    // name of the task
			reply.Number = numberOfMapTask                          // total number of tasks
			reply.Type = mapType                                    // type of task, of course it is map
			reply.PartitionNumber = c.numbeOfReduce                 // number of partition to assign
			reply.MapAddresses = locations                          // address book
			c.mapTasks[mapTask].assignedWorker = args.WorkerAddress //
			c.cond.L.Unlock()                                       // complete the critical selection
			return nil
		}
	}
	// check reduce task
	if c.reduceRemaining != 0 {
		reduceTask, numberOfReduceTask := c.GetReduceTask()
		for reduceTask == "" {
			if c.reduceRemaining == 0 {
				reply.Name = ""
				reply.Number = -1
				reply.Type = waitType
				reply.MapAddresses = locations
				c.cond.L.Unlock()
				return nil
			}
			c.cond.Wait()
			reduceTask, numberOfReduceTask = c.GetReduceTask()
		}
		// dont have to fetch the queue because reduce task can be taken from output of map
		reply.Name = reduceTask
		reply.Number = numberOfReduceTask
		reply.MapAddresses = locations
		reply.Type = reduceType
		c.cond.L.Unlock()
		return nil
	}

	c.cond.L.Unlock()
	return errors.New("all tasks are completed, no more remaining")
}

// update the status for map or reduce task
func (c *Coordinator) UpdateTaskStatus(args *UpdateTaskStatusArgs, reply *UpdateTaskStatusReply) error {
	c.cond.L.Lock()
	defer c.cond.L.Unlock()

	if args.Type == mapType {
		task, ok := c.mapTasks[args.Name]
		if !ok {
			return nil
		}
		task.status = completed
		c.mapRemaining -= 1
	} else {
		task, ok := c.reduceTasks[args.Name]
		if !ok {
			return nil
		}
		task.status = completed
		c.reduceRemaining -= 1
	}
	return nil
}

func (c *Coordinator) HealthMonitor() {
	for {
		time.Sleep(1 * time.Second)
		c.cond.L.Lock()

		for addr, worker := range c.workerMap {
			if time.Since(worker.LastReportedTime) > 10*time.Second {
				HardReset(c, addr)
				SoftReset(c, addr)
				delete(c.workerMap, addr)
			}
		}
		c.cond.L.Unlock()
	}
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
						c.mapTasks[task].assignedWorker = ""
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
						c.reduceTasks[task].assignedWorker = ""
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
func (c *Coordinator) HealthCheck(args *HealthCheckArgs, reply *HealthCheckReply) error {
	c.cond.L.Lock()
	defer c.cond.L.Unlock()

	if worker, ok := c.workerMap[args.WorkerAddress]; ok {
		worker.LastReportedTime = time.Now()
		worker.WorkerDown = false
	} else {
		c.workerMap[args.WorkerAddress] = &WorkerInfor{
			WorkerAddress:    args.WorkerAddress,
			LastReportedTime: time.Now(),
			WorkerDown:       false,
		}
	}

	// log.Printf("new worker sent heartbeat, address %s, time: %v", c.workerMap[args.WorkerAddress].WorkerAddress, c.workerMap[args.WorkerAddress].LastReportedTime)

	reply.Acknowledge = true
	return nil
}

func HardReset(c *Coordinator, failedWorker string) {
	for _, task := range c.mapTasks {
		if task.assignedWorker == failedWorker {
			prevStatus := task.status
			task.status = unstarted
			task.assignedWorker = ""
			task.startTime = time.Time{}
			if prevStatus == completed {
				c.mapRemaining++
			}
		}
	}
}

func SoftReset(c *Coordinator, failedWorker string) {
	for _, task := range c.reduceTasks {
		if task.assignedWorker == failedWorker {
			task.status = unstarted
			task.assignedWorker = ""
			task.startTime = time.Time{}
		}
	}
}

func (c *Coordinator) ReportFailure(args *FailedTaskReportArgs, reply *FailedTaskReportReply) error {
	c.cond.L.Lock()
	defer c.cond.L.Unlock()

	failedWorker := args.WorkerAddress

	if w, ok := c.workerMap[failedWorker]; ok {
		w.WorkerDown = true
	}

	HardReset(c, failedWorker) // for map task
	SoftReset(c, failedWorker)

	c.cond.Broadcast()
	return nil
}

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
		mapTasks:        mapTask,
		reduceTasks:     reduceTask,
		mapRemaining:    len(files),
		reduceRemaining: nReduce,
		numbeOfReduce:   nReduce,
		cond:            cond,
		workerMap:       make(map[string]*WorkerInfor),
	}

	// Your code here.

	// start a new rountine for the rescheduler (constanly check for exceeded time task independently)
	go c.Rescheduler()
	go c.HealthMonitor()

	c.server()
	return &c
}

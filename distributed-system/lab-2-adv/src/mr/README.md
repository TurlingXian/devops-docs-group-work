>[! Requirements]
>Your program now runs on a single machine. Now, distribute your program to run on multiple machines (we recommend following the [MapReduce paper](http://research.google.com/archive/mapreduce-osdi04.pdf)
Your choice on the cloud, lab computers, or your own machines. You are not allowed to use a shared file system such as EFS.

For full points, your solution should:

- Store intermediate files at the worker whom produced them.
- Use RPC (or similar) to communicate between processes.
- Handle worker failures and delays in a sane way. Try recovering from errors, but also not doing excessive duplicate work, as for the tests in the basic part.
- At least detect a coordinator failure.

# 1. How to run?

Need at least 3 machines for better illustration. 1 for coordinator and 2 for workers. Copy all the code, then compile with `-race` flag to detect race condition.
```bash
~\lab2\src\main
# The working directory should contains these files
❯ ls
diskvd.go  mrcoordinator     mrworker     pg-being_ernest.txt  pg-huckleberry_finn.txt  test-mr-many.sh
lockc.go   mrcoordinator.go  mrworker.go  pg-dorian_gray.txt   pg-metamorphosis.txt     test-mr.sh
lockd.go   mrsequential      pbc.go       pg-frankenstein.txt  pg-sherlock_holmes.txt   viewd.go
mr-tmp     mrsequential.go   pbd.go       pg-grimm.txt         pg-tom_sawyer.txt
# build a fresh version of worker, coordinator and rpc
go build -race -buildmode=plugin ../mrapps/wc.go
```
To be able to build with the `-race` detector, using:
```bash
export GORACE=1
```
Run the coordinator in a machine:
```Bash
# specify the address of the coordinator (within LAN)
export COORDINATOR_HOST=<Private_ip>:<Port>
# run the coordinator with tasks
go run -race mrcoordinator.go pg-*.txt
```
With the workers:
```Bash
# specify the address of the coordinator (within LAN), used the coordinator IP
export COORDINATOR_HOST=<Private_ip>:<Port>
# run the worker
go run -race mrworker.go wc.so
```
The Coordinator will distribute tasks for the worker, worker completes the task, then uploads these results to coordinator, which will assemble all the files to product a single file with name `mr-wc-all`. This file can be used in the comparison with the result produced from `go run mrsequential.go wc.so pg*.txt`.

# 2. Details
## 2.1. A heartbeat approach
Each worker will send heartbeat to coordinator frequently. On the coordinator side, there is a function to handle the Health Check. To do this, coordinator must have a map, map a worker name with that worker's information:
```Go
type WorkerInfor struct {
    WorkerAddress    string
    LastReportedTime time.Time
    WorkerDown       bool
}
```
Each worker has some essential information: the address (since we need to expose the address for the reduce phase), the last time it reported health to coordinator and a True/False indication if that worker is operating or not.
Upon receiving a heartbeat, the coordinator will update the `LastReportedTime` if that worker is already in list, or registering a new one if not:
```Go
func (c *Coordinator) HealthCheck(args *HealthCheckArgs, reply *HealthCheckReply) error {
    c.cond.L.Lock()
    defer c.cond.L.Unlock()

    if worker, ok := c.workerMap[args.WorkerAddress]; ok {
        worker.LastReportedTime = time.Now()
        worker.WorkerDown = false
    } else {
        c.workerMap[args.WorkerAddress] = &WorkerInfor{
            WorkerAddress:    args.WorkerAddress,
            LastReportedTime: time.Now(),
            WorkerDown:       false,
        }
    }
    reply.Acknowledge = true
    return nil
}
```
For this purpose, there are 2 new structs (RPC):
```Go
type HealthCheckArgs struct {
    WorkerAddress string
    LastUpTime    time.Time
}
 
type HealthCheckReply struct {
    Acknowledge bool
}
```
Each time worker tries to send the heartbeat, it relies on the Acknowledge to determine if it reported successfully or not.
```Go
func CallHealthCheck(addr string) (bool, error) {
    args := HealthCheckArgs{
        WorkerAddress: addr,
        LastUpTime:    time.Now(),

    }
    reply := HealthCheckReply{}
    ok := call("Coordinator.HealthCheck", &args, &reply)
    if ok {
        return reply.Acknowledge, nil
    } else {
        return false, errors.New("call falied")
    }
}
```
## 2.2 Handle the files in the distributed environment
In this model, map and reduce worker are not in the same machine, so each worker must expose itself as a fileserver, sends its address when replying to the coordinator (construct an address book). With this additional information, when reduce phase is started, the reduce worker can know where to find the file.
First, when a worker started to work, also exposed itself (and also performed regular health check with coordinator):
```Go
func Worker(mapf func(string, string) []KeyValue,
    reducef func(string, []string) string) {
    // exposed self
    workerAddress := StartHTTPFileServer(".")
    if workerAddress == "" {
        return
    }
    // regular health check
    go func() {
        ticker := time.NewTicker(500 * time.Millisecond)
        defer ticker.Stop()
        for range ticker.C {
            _, err := CallHealthCheck(workerAddress)
            if err != nil {
                log.Fatal(err)
            }
        }
    }()
```
Because the coordinator must know workers that executed map tasks, the worker also needs to contain its address in the Update method `CallUpdateTaskStatus(mapType, rep.Name, workerAddress)`. The reply should contain these information:
In RPC
```Go
type GetTaskReply struct {
    Name            string // the name of file that acts as the input
    Number          int    // task number (to be refferred each phases, not to be confused with partition ID to put to reduce)
    PartitionNumber int    // partition number, to match with which reducer will take it
    Type            TaskType
    MapAddresses    []string // used to store all map workers addresses
}
```
In the `CallUpdateTaskStatus`:
```Go
func CallUpdateTaskStatus(tasktype TaskType, name string, workeraddress string) error {
    args := UpdateTaskStatusArgs{
        Name:          name,
        Type:          tasktype,
        WorkerAddress: workeraddress,
    }
    reply := UpdateTaskStatusReply{}
    ok := call("Coordinator.UpdateTaskStatus", &args, &reply)
    if ok {
        // log.Printf("call with these args: %s, %s, %s", name, tasktype, workeraddress)
        return nil
    } else {
        return errors.New("call failed")
    }
}
```
And the Coordinator also handles the worker:
```Go
func (c *Coordinator) GetTask(args *GetTaskArgs, reply *GetTaskReply) error {
    c.cond.L.Lock() // lock the conditional variable when accessing shared variable
    // in the get task, include a map of dictonary book
    locations := make([]string, len(c.mapTasks))
    for name, taskMetaData := range c.mapTasks {
        if name != "" && taskMetaData.assignedWorker != "" {
            locations[taskMetaData.number] = taskMetaData.assignedWorker
        }
    }
    ...
```
To reduced the update overhead, each time a worker requests a task successfully, the coordinator also update the assigned worker (to know a tasks executed by which worker), included a list in the reply.
```Go
// inside MapType
...
            reply.MapAddresses = locations                          // address book
            c.mapTasks[mapTask].assignedWorker = args.WorkerAddress //
...
```
## 2.3. Fetching files from multiple sources.
In a scenario, if there are 100 workers, and they are reused for reduce task, maybe the reducer must loop through all 100 workers, to decrease time, using a remote fetching methods with multiple routines:
```Go
func FetchData(mapWorkerAddress []string, partition int) ([]KeyValue, error) {
    var wg sync.WaitGroup
    var mu sync.Mutex

    fetchedData := make([]KeyValue, 0)
    errChan := make(chan error, len(mapWorkerAddress))

    client := http.Client{
        Timeout: 2 * time.Second,
    }

    for i, address := range mapWorkerAddress {
        if address == "" {
            continue
        }
        wg.Add(1)
        go func(index int, addr string) {
            defer wg.Done()
            filename := fmt.Sprintf("mr-%d-%d", index, partition)
            fileurl := fmt.Sprintf("%s/%s", addr, filename)
            resp, err := client.Get(fileurl)
            if err != nil {
                go func(badAddr string) {
                    log.Printf("Failed worker %s", badAddr)
                    CallFailureTask(badAddr)
                }(addr)
                errChan <- fmt.Errorf("map worker %s unreachable", addr)
                return
            }

            defer resp.Body.Close()
            if resp.StatusCode != http.StatusOK {
                go func(badAddr string) {
                    log.Printf("Failed worker %s", badAddr)
                    CallFailureTask(badAddr)
                }(addr)
                errChan <- fmt.Errorf("map worker %s returned %d", addr, resp.StatusCode)
                return
            }

            var localData []KeyValue
            decoder := json.NewDecoder(resp.Body)
            for {
                var kv KeyValue
                if err := decoder.Decode(&kv); err != nil {
                    if err == io.EOF {
                        break
                    }
                    go func(badAddr string) {
                        log.Printf("Failed worker %s", badAddr)
                        CallFailureTask(badAddr)
                    }(addr)
                    errChan <- err
                    return
                }
                localData = append(localData, kv)
            }
            mu.Lock()
            fetchedData = append(fetchedData, localData...)
            mu.Unlock()
        }(i, address)
    }

    wg.Wait()
    close(errChan)
    for err := range errChan {
        if err != nil {
            return nil, err
        }
    }
    return fetchedData, nil
}
```
In this method, if fetch file failed, meaning the worker contains necessary files is down. Then, it should be reported to coordinator, then, coordinator will reset all the tasks that are associated with that worker (through property `assignedWorker`).
```Go
// failure call
func CallFailureTask(addr string) (bool, error) {
    args := FailedTaskReportArgs{
        WorkerAddress: addr,
    }

    reply := FailedTaskReportReply{}
    ok := call("Coordinator.ReportFailure", &args, &reply)
    if ok {
        return reply.Acknowledge, nil
    } else {
        return false, errors.New("call failed")
    }
}
```
Coordinator handles failure:
```Go
func (c *Coordinator) ReportFailure(args *FailedTaskReportArgs, reply *FailedTaskReportReply) error {
    c.cond.L.Lock()
    defer c.cond.L.Unlock()

    failedWorker := args.WorkerAddress
    if w, ok := c.workerMap[failedWorker]; ok {
        w.WorkerDown = true
    }
    log.Printf("Worker reported down %s", failedWorker)
    HardReset(c, failedWorker) // for map task
    SoftReset(c, failedWorker) // for reduce task
    c.cond.Broadcast()
    return nil
}
```
The hard is for map, if status == `inprogress`, only set to `unstarted`, but if it was completed ago, reset and also plus 1 to the total map remaining task. But the soft reset is for reduce, only set to `unstarted` and then wait to be rescheduled later.
## 2.4. Running both the Reschedule and Health Check
```Go
func MakeCoordinator(files []string, nReduce int) *Coordinator {
...
    go c.Rescheduler()
    go c.HealthMonitor()
...
}
```
While the `Rescheduler` takes care the task that runs slowly (more than 10 secs), the `HealthMonitor` will reset the tasks if a worker is down (not hearing heartbeat more than 4 secs):
```Go
func (c *Coordinator) HealthMonitor() {
    for {
        time.Sleep(500 * time.Millisecond)
        c.cond.L.Lock()

        for addr, worker := range c.workerMap {
            if time.Since(worker.LastReportedTime) > 4*time.Second {
                log.Printf("Worker down %s", addr)
                HardReset(c, addr)
                SoftReset(c, addr)
                delete(c.workerMap, addr)

                c.cond.Broadcast()
            }
        }
        c.cond.L.Unlock()
    }
}
```

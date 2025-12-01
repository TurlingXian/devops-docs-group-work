package mr

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// for advance feature
const WorkerCommonPort = 30022

// borrow from mrapps
type ByKey []KeyValue

func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

// Map functions return a slice of KeyValue.
type KeyValue struct {
	Key   string
	Value string
}

// helper function to start a fileserver
func StartHTTPFileServer(rootDirectory string) string {
	// change to dynamicall assigned port (for local test)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", WorkerCommonPort))
	if err != nil {
		log.Fatalf("Error encountered: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	ip, err := GetServerAddress()

	serverAddress := fmt.Sprintf("http://%s:%d", ip, port)
	if err != nil {
		log.Fatalf("Error encountered: %v", err)
	}

	log.Printf("Server information: %s, %d", ip, port)
	go http.Serve(listener, http.FileServer(http.Dir(rootDirectory)))

	return serverAddress
}

// helper function to get the worker 'current' address and send it along with the reply
func GetServerAddress() (myAdress string, error error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String(), nil
		}
	}
	return "", errors.New("cannot get address")
}

// self check the map exposed
func ProbleFileExposed(addr string, filename string) error {
	url := fmt.Sprintf("%s/%s", addr, filename)
	client := http.Client{
		Timeout: 100 * time.Millisecond,
	}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("[Self-check] Failed, could not connect to self: %s, error %v", addr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("[Self-check] Failed, either the file has been moved or insufficent permisison, file %s, error %v", filename, resp.StatusCode)
	}
	return nil
}

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

// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

// main/mrworker.go calls this function.
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	workerAddress := StartHTTPFileServer(".")
	if workerAddress == "" {
		return
	}

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

	for {
		rep, err := CallGetTask(workerAddress)
		if err != nil {
			// log.Fatal(err)
			// log.Printf("call failed due to %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		if rep.Type == mapType {
			// if get a map task, execute then call update
			log.Printf("Got map task with information %s, %d, %d. Self %s", rep.Name, rep.Number, rep.PartitionNumber, workerAddress)
			ExecuteMapTask(rep.Name, rep.Number, rep.PartitionNumber, mapf)
			log.Printf("Completed, update with information %s, %s", rep.Name, workerAddress)
			CallUpdateTaskStatus(mapType, rep.Name, workerAddress)
			// time.Sleep(10 * time.Millisecond)
		} else {
			for _, s := range rep.MapAddresses {
				log.Printf("Address of previous worker: %s", s)
			}
			log.Printf("Got reduce task with information %s", rep.Name)
			err := ExecuteReduceTask(rep.Number, reducef, rep.MapAddresses)
			if err == nil {
				log.Printf("Completed, update with information %s", rep.Name)
				CallUpdateTaskStatus(reduceType, rep.Name, "")
				// time.Sleep(10 * time.Millisecond)
			} else {
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
}

// function for map worker
func ExecuteMapTask(filename string, mapNumber, numberofReduce int, mapf func(string, string) []KeyValue) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("Cannot open %v", filename)
	}
	content, err := io.ReadAll(file)
	if err != nil {
		log.Fatalf("Cannot read %v", filename)
	}
	file.Close()
	initVal := mapf(filename, string(content)) // map the filename with its content
	mp := map[int]*os.File{}                   // map of output result (cache)

	// In ExecuteMapTask
	for i := 0; i < numberofReduce; i++ {
		filename := fmt.Sprintf("mr-%d-%d", mapNumber, i)
		// Create file immediately, even if we write nothing to it later
		file, _ := os.Create(filename)
		mp[i] = file
	}

	for _, kv := range initVal {
		// check for each "word"
		currentParition := ihash(kv.Key) % numberofReduce
		f, ok := mp[currentParition]
		if !ok {
			if err != nil {
				log.Fatal(err)
			}
		}
		kvj, _ := json.Marshal(kv)
		fmt.Fprintf(f, "%s\n", kvj)
	}

	// rename for the reduce phase
	for rNum, f := range mp {
		f.Close()
		os.Rename(f.Name(), fmt.Sprintf("mr-%d-%d", mapNumber, rNum))
	}
}

// function for reduce worker
func ExecuteReduceTask(partitionNumber int, reducef func(string, []string) string, mapWorkerAddress []string) error {
	data, err := FetchData(mapWorkerAddress, partitionNumber)
	if err != nil {
		log.Printf("[ERROR][Worker] FetchData failed: %v", err)
		return err
	}

	sort.Sort(ByKey(data))

	oname := fmt.Sprintf("mr-out-%d", partitionNumber)
	ofile, _ := os.Create(oname)

	i := 0
	for i < len(data) {
		j := i + 1
		for j < len(data) && data[j].Key == data[i].Key {
			j++
		}
		values := []string{}
		for k := i; k < j; k++ {
			values = append(values, data[k].Value)
		}
		output := reducef(data[i].Key, values)

		fmt.Fprintf(ofile, "%v %v\n", data[i].Key, output)
		i = j
	}
	ofile.Close()

	return nil
}

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

// health report
func CallHealthCheck(addr string) (bool, error) {
	args := HealthCheckArgs{
		WorkerAddress: addr,
		LastUpTime:    time.Now(),
	}
	reply := HealthCheckReply{}
	ok := call("Coordinator.HealthCheck", &args, &reply)
	if ok {
		return reply.Acknowledge, nil
	} else {
		return false, errors.New("call falied")
	}
}

// function call to get a task from coordinator
func CallGetTask(addr string) (*GetTaskReply, error) {
	args := GetTaskArgs{
		WorkerAddress: addr,
	}
	reply := GetTaskReply{}

	ok := call("Coordinator.GetTask", &args, &reply)
	if ok {
		// get response success
		// fmt.Printf("reply.Name '%v', reply.Type '%v'\n", reply.Name, reply.Type)
		return &reply, nil
	} else {
		// some errors happened
		return nil, errors.New("call failed")
	}
}

// function to update a task status (done, timeout,...)
func CallUpdateTaskStatus(tasktype TaskType, name string, workeraddress string) error {
	args := UpdateTaskStatusArgs{
		Name:          name,
		Type:          tasktype,
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

// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
func call(rpcname string, args interface{}, reply interface{}) bool {
	// // c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	// sockname := coordinatorSock()
	// c, err := rpc.DialHTTP("unix", sockname)
	// if err != nil {
	// 	log.Fatal("dialing:", err)
	// }
	// defer c.Close()

	// err = c.Call(rpcname, args, reply)
	// if err == nil {
	// 	return true
	// }

	// fmt.Println(err)
	// // log.Fatalf(err.Error())
	// return false
	sockname := coordinatorSock()

	var c *rpc.Client
	var err error

	if strings.Contains(sockname, ":") {
		c, err = rpc.DialHTTP("tcp", sockname)
	} else {
		c, err = rpc.DialHTTP("unix", sockname)
	}

	if err != nil {
		log.Fatal("[ERROR] Dialing error:", err)
	}

	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)

	return false
}

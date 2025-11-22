package mr

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// for advance feature
const directoryPath = "."

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
	listener, err := net.Listen("tcp", ":0")
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

	// Your worker implementation here.

	// start the file server first, then take the address string

	// go func() {
	// 	if err := StartHTTPFileServer(directoryPath, defaultFilePort); err != nil {
	// 		fmt.Printf("Server failed: %v", err)
	// 		os.Exit(1)
	// 	}
	// }()

	// workerAddress, err := GetServerAddress()
	// if workerAddress == "" {
	// 	log.Fatalf("Cannot get the worker's address: %v", err)
	// }
	// uncomment to send the Example RPC to the coordinator.
	// CallExample()
	workerAddress := StartHTTPFileServer(directoryPath)
	log.Printf("Worker file server started at %s", workerAddress)

	for {
		rep, err := CallGetTask()
		if err != nil {
			log.Fatal(err)
		}
		switch rep.Type {
		case mapType:
			ExecuteMapTask(rep.Name, rep.Number, rep.PartitionNumber, mapf)
			CallUpdateTaskStatus(mapType, rep.Name, workerAddress)
		case reduceType:
			err := ExecuteReduceTask(rep.Number, reducef, rep.MapAddresses)
			if err != nil {
				time.Sleep(1 * time.Second) // simply wait if err, since map must be rerun
			} else {
				CallUpdateTaskStatus(reduceType, rep.Name, "")
			}
		case waitType: // map worker wait for reduce worker to take the file (to not forcefully exist)
			time.Sleep(1 * time.Second)
		}
		// if rep.Type == mapType {
		// 	// if get a map task, execute then call update
		// 	ExecuteMapTask(rep.Name, rep.Number, rep.PartitionNumber, mapf)
		// 	CallUpdateTaskStatus(mapType, rep.Name, workerAddress)
		// } else {
		// 	ExecuteReduceTask(rep.Number, reducef, rep.MapAddresses)
		// 	CallUpdateTaskStatus(reduceType, rep.Name, "")
		// }
	}
}

// helper function to look for all the matching file (reduce looks for files created by map)
func WalkDir(root string, reduceNumber int) ([]string, error) {
	var files []string
	// search for partition number
	pattern := fmt.Sprintf(`mr-\d+-%d$`, reduceNumber)
	reg, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err // error happened, return NULL
	}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}
		if reg.Match([]byte(d.Name())) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
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

	for _, kv := range initVal {
		// check for each "word"
		currentParition := ihash(kv.Key) % numberofReduce // number of parition (percisely)
		f, ok := mp[currentParition]
		if !ok {
			// create new "bucket" if the word is not existed
			f, err = os.CreateTemp("", "tmp")
			mp[currentParition] = f
			if err != nil {
				log.Fatal(err)
			}
		}
		kvj, _ := json.Marshal(kv)
		// fmt.Fprint(f, "%v\n", kvj)
		fmt.Fprintf(f, "%s\n", kvj)
	}

	// rename for the reduce phase, this can be caused a false error, with smaller keys than the number of reduce
	for i := 0; i < numberofReduce; i++ {
		finalName := fmt.Sprintf("mr-%d-%d", mapNumber, i)
		f, ok := mp[i]
		if ok {
			f.Close()
			os.Rename(f.Name(), finalName) // change name if can open file
		} else {
			// empty partition because of fewer keys than actual number of reduce workers
			emptyFile, err := os.Create(finalName)
			if err != nil {
				log.Fatal(err)
			}
			emptyFile.Close()
		}
	}
}

// function for reduce worker
func ExecuteReduceTask(partitionNumber int, reducef func(string, []string) string, mapWorkerAddress []string) error {
	// fetch the filename (change to remote fetching)
	// filenames, _ := WalkDir("./", partitionNumber) // look for current directory of all file with reduceNumber pattern
	data := make([]KeyValue, 0)
	// check the failed first (before setup all the inputs)
	for mapWorkerID, workerAddress := range mapWorkerAddress {
		filename := fmt.Sprintf("mr-%d-%d", mapWorkerID, partitionNumber)
		fileURL := fmt.Sprintf("%s/%s", workerAddress, filename)

		resp, err := http.Get(fileURL)
		if err != nil {
			// log.Printf("failed to read from %s: %v", fileURL, err)
			// os.Exit(1) // cannot continue since data is corrupt (for now)
			CallReportFailureMapTask(mapWorkerID)
			return fmt.Errorf("map task ID %d unreachable", mapWorkerID)
		}
		if resp.StatusCode != http.StatusOK {
			// We got a 404 or 500. Do NOT parse this as JSON! (since it will generate "unexpected" letter p)
			log.Printf("Worker at %s returned status %d for file %s", workerAddress, resp.StatusCode, filename)
			resp.Body.Close()
			CallReportFailureMapTask(mapWorkerID)
			return fmt.Errorf("missing expected data from task %d", mapWorkerID)
		}
		defer resp.Body.Close()

		content, err := io.ReadAll(resp.Body)
		if err != nil { // failed when read (at the moddle of reading stuff)
			// log.Fatalf("Cannot read from %v, error %s", resp, err)
			// os.Exit(1)
			log.Printf("Read error from %s: %v", fileURL, err)
			CallReportFailureMapTask(mapWorkerID)
			return fmt.Errorf("read error from map task %d", mapWorkerID)
		}
		kvstrings := strings.Split(string(content), "\n")
		for _, kvstring := range kvstrings {
			// trimmed
			trimmed := strings.TrimSpace(kvstring)
			if len(trimmed) == 0 {
				continue
			}
			kv := KeyValue{}
			err := json.Unmarshal([]byte(trimmed), &kv)
			if err != nil {
				// log.Fatalf("cannot unmarshal %v, error %s", filename, err)
				// os.Exit(1)
				log.Printf("Data corruption detected in %s (Task %d). Reporting failure.", filename, mapWorkerID)
				CallReportFailureMapTask(mapWorkerID)
				return fmt.Errorf("corrupted data from map task %d", mapWorkerID)
			}
			data = append(data, kv)
		}
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

// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
// func CallExample() {

// 	// declare an argument structure.
// 	args := ExampleArgs{}

// 	// fill in the argument(s).
// 	args.X = 99

// 	// declare a reply structure.
// 	reply := ExampleReply{}

// 	// send the RPC request, wait for the reply.
// 	// the "Coordinator.Example" tells the
// 	// receiving server that we'd like to call
// 	// the Example() method of struct Coordinator.
// 	ok := call("Coordinator.Example", &args, &reply)
// 	if ok {
// 		// reply.Y should be 100.
// 		fmt.Printf("reply.Y %v\n", reply.Y)
// 	} else {
// 		fmt.Printf("call failed!\n")
// 	}
// }

// function call to get a task from coordinator
func CallGetTask() (*GetTaskReply, error) {
	args := GetTaskArgs{}
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
		log.Printf("call with these args: %s, %s, %s", name, tasktype, workeraddress)
		return nil
	} else {
		return errors.New("call failed")
	}
}

// call to report failed task
func CallReportFailureMapTask(mapIndex int) {
	args := ReportFailureArgs{
		MapTaskIndex: mapIndex,
	}
	reply := ReportFailureReply{}
	call("Coordinator.ReportMapWorkerFailure", &args, &reply)
}

// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	// log.Fatalf(err.Error())
	return false
}

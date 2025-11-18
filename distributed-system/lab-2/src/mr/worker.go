package mr

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"log"
	"net/rpc"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

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

	// uncomment to send the Example RPC to the coordinator.
	// CallExample()
	for {
		rep, err := CallGetTask()
		if err != nil {
			log.Fatal(err)
		}

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
		currentParition := ihash(kv.Key) % numberofReduce
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
		fmt.Fprint(f, "%s\n", kvj)
	}

	// rename for the reduce phase
	for rNum, f := range mp {
		os.Rename(f.Name(), fmt.Sprintf("mr-%d-%d", mapNumber, rNum))
	}
}

// function for reduce worker
func ExecuteReduceTask(partitionNumber int, reducef func(string, []string) string) {
	// fetch the filename
	filenames, _ := WalkDir("./", partitionNumber) // look for current directory of all file with reduceNumber pattern
	data := make([]KeyValue, 0)
	for _, filename := range filenames {
		file, err := os.Open(filename)
		if err != nil {
			log.Fatalf("Cannot open file %v, error %s", filename, err)
		}
		content, err := io.ReadAll(file)
		if err != nil {
			log.Fatalf("Cannot read %v, error %s", filename, err)
		}
		file.Close()

		kvstrings := strings.Split(string(content), "\n")
		kv := KeyValue{}
		for _, kvstring := range kvstrings[:len(kvstrings)-1] {
			err := json.Unmarshal([]byte(kvstring), &kv)
			if err != nil {
				log.Fatalf("Cannot unmarshal %v, error %s", filename, err)
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
}

// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	// the "Coordinator.Example" tells the
	// receiving server that we'd like to call
	// the Example() method of struct Coordinator.
	ok := call("Coordinator.Example", &args, &reply)
	if ok {
		// reply.Y should be 100.
		fmt.Printf("reply.Y %v\n", reply.Y)
	} else {
		fmt.Printf("call failed!\n")
	}
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
	return false
}

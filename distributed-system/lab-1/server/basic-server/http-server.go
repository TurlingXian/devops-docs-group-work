package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const MAX_CONNECTION = 10

var supportedTypes = map[string]string{
	".html": "text/html",
	".txt":  "text/plain",
	".gif":  "image/gif",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".css":  "text/css",
}

type server struct {
	// using atomic as a method for synchronization (to avoid race condition, package: sync/atomic)
	connectionCount atomic.Int32
	// wait group for synchronization method, since this servers should handle connections concurrently (and also accept them as well)
	wg sync.WaitGroup
	// listener socket
	listener net.Listener
	// shutdown signal
	shutdown chan struct{}
	// connection pooling - no need for additional connection pool
	connection chan net.Conn
}

func newServer(address string) (*server, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on address %s : %w", address, err)
	}

	return &server{
		listener:   listener,
		shutdown:   make(chan struct{}),
		connection: make(chan net.Conn),
	}, nil
}

// handle the "accept connection phase"
func (s *server) acceptConnections() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				log.Println("acceptConnections shutting down.")
				return
			default:
				// Some other accept error
				log.Printf("Listener accept error: %v", err)
				continue
			}
		}
		s.connection <- conn
	}
}

func (s *server) worker() {
	defer s.wg.Done()

	for conn := range s.connection {
		s.handleConnection(conn)
	}
}

func (s *server) handleConnection(conn net.Conn) {
	defer conn.Close()

	currentConn := s.connectionCount.Add(1)
	log.Printf("Total active connection is: %d", currentConn)
	_, err := fmt.Fprintf(conn, "Welcome! Total active: %d\n", currentConn)
	if err != nil {
		log.Printf("Error writing welcome message: %v", err)
		// Don't continue if we can't even write to the client
		s.connectionCount.Add(-1) // Decrement since we're closing early
		return
	}

	log.Println("Accepted a connection")
	reader := bufio.NewReader(conn)

	req, err := http.ReadRequest(reader)

	if err != nil {
		log.Printf("Malformed request from the client")
		s.handleError(conn, http.StatusBadRequest)
		return
	}

	switch req.Method {
	case "GET":
		s.handleGet(conn, req)
	case "POST":
		s.handlePost(conn, req)
	default:
		s.handleError(conn, http.StatusNotImplemented)
	}

	remainingConn := s.connectionCount.Add(-1)
	log.Printf("Handling a connection completed, remaining: %d", remainingConn)
}

func (s *server) handleGet(conn net.Conn, req *http.Request) {
	const staticDir = "static"
	path := filepath.Join(staticDir, filepath.Clean(req.URL.Path))

	if !strings.HasPrefix(path, staticDir) {
		s.handleError(conn, http.StatusBadRequest)
		return
	}

	fileExt := filepath.Ext(path)
	fileType, supported := supportedTypes[fileExt]
	if !supported {
		log.Printf("Bad request: unsupported extension %s", fileExt)
		s.handleError(conn, http.StatusBadRequest)
		return
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Not found: %s", path)
			s.handleError(conn, http.StatusNotFound)
		} else {
			log.Printf("Internal error: %v", err)
			s.handleError(conn, http.StatusInternalServerError)
		}
		return
	}

	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		log.Printf("Internal error while handling the file: %v", err)
		s.handleError(conn, http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\n")
	// Headers
	fmt.Fprintf(conn, "Content-Type: %s\r\n", fileType)
	fmt.Fprintf(conn, "Content-Length: %d\r\n", stat.Size())
	// End of headers (blank line)
	fmt.Fprintf(conn, "\r\n")

	if _, err := io.Copy(conn, file); err != nil {
		log.Printf("Error while writing the file body: %v", err)
	}
}

func (s *server) handlePost(conn net.Conn, req *http.Request) {
	const uploadDir = "uploads"
	if _, err := os.Stat(uploadDir); os.IsNotExist(err) {
		if err := os.Mkdir(uploadDir, 0755); err != nil {
			log.Printf("Internal error while creating storage: %v", err)
			s.handleError(conn, http.StatusInternalServerError)
			return
		}
	}

	path := filepath.Join(uploadDir, filepath.Clean(req.URL.Path))

	if !strings.HasPrefix(path, uploadDir) {
		s.handleError(conn, http.StatusBadRequest)
		return
	}

	file, err := os.Create(path)
	if err != nil {
		log.Printf("Internal error creating file: %v", err)
		s.handleError(conn, http.StatusInternalServerError)
		return
	}

	defer file.Close()

	if _, err := io.Copy(file, req.Body); err != nil {
		log.Printf("Internal error writing file: %v", err)
		s.handleError(conn, http.StatusInternalServerError) // 500
		return
	}

	log.Printf("File uploaded: %s", path)

	// Status Line
	fmt.Fprintf(conn, "HTTP/1.1 201 Created\r\n")

	// Headers
	fmt.Fprintf(conn, "Content-Type: text/plain\r\n")
	// The 'Location' header tells the client where the new resource is
	fmt.Fprintf(conn, "Location: %s\r\n", req.URL.Path)

	// End of headers
	fmt.Fprintf(conn, "\r\n")

	// Body
	fmt.Fprintln(conn, "File successfully uploaded.")

}

func (s *server) handleError(conn net.Conn, statusCode int) {
	statusText := http.StatusText(statusCode)

	// Status Line
	fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\n", statusCode, statusText)
	// Headers
	fmt.Fprintf(conn, "Content-Type: text/plain\r\n")
	fmt.Fprintf(conn, "Content-Length: %d\r\n", len(statusText))
	// End of headers
	fmt.Fprintf(conn, "\r\n")
	// Body
	fmt.Fprintln(conn, statusText)
}

func (s *server) Start() {
	s.wg.Add(1)
	go s.acceptConnections()

	s.wg.Add(MAX_CONNECTION)
	for i := 0; i < MAX_CONNECTION; i++ {
		go s.worker()
	}
}

func (s *server) Stop() {
	close(s.shutdown)
	s.listener.Close()

	close(s.connection)

	done := make(chan struct{})
	go func() {
		// This now correctly waits for all 11 goroutines
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All goroutines shut down gracefully.")
		return
	case <-time.After(5 * time.Second):
		log.Println("Time out waiting remaining connections to be finished.")
		return
	}
}

func main() {
	arguments := os.Args
	PORT := ""
	if len(arguments) == 1 {
		log.Println("A port was not specified, using a random port for the server!")
		PORT += ":0"
	} else {
		PORT = PORT + ":" + arguments[1]
	}

	s, error := newServer(PORT)
	if error != nil {
		log.Println(error)
		os.Exit(1)
	}

	log.Printf("Server was started on PORT: %d", s.listener.Addr().(*net.TCPAddr).Port)

	s.Start()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down the server ...")
	s.Stop()
	log.Println("Server stopped.")
}

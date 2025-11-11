package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

// reuse the limit of 10 from the "Basic Part"
const MAX_CLIENTS = 10

// --- Main Proxy Server ---

func main() {
	// 1. Check for command-line argument (Requirement: take port as argument)
	if len(os.Args) != 2 {
		fmt.Println("Usage: ./proxy <port>")
		os.Exit(1)
	}
	port := os.Args[1]

	// 2. Create the TCP listener (Requirement: Use 'net' package)
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}
	defer listener.Close()
	log.Printf("Proxy server listening on port %s...", port)

	// 3. Create semaphore to limit concurrency
	semaphore := make(chan struct{}, MAX_CLIENTS)

	// 4. The main accept loop
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		semaphore <- struct{}{} // Acquire a slot

		// Requirement: Spawn a new Go routine for each new request
		go handleConnection(conn, semaphore)
	}
}

// --- Connection Handling ---

func handleConnection(conn net.Conn, semaphore chan struct{}) {
	defer conn.Close()
	defer func() {
		<-semaphore // Release the slot
	}()

	reader := bufio.NewReader(conn)

	// Requirement: Use 'net/http' ONLY for parsing
	req, err := http.ReadRequest(reader)
	if err != nil {
		if err != io.EOF {
			log.Printf("Error reading request: %v", err)
			sendErrorResponse(conn, 400, "Bad Request", "Your request could not be parsed.")
		}
		return
	}
	defer req.Body.Close()

	log.Printf("Proxying: %s %s", req.Method, req.URL.String())

	// Requirement: Only implement GET, other methods return 501
	if req.Method != "GET" {
		sendErrorResponse(conn, 501, "Not Implemented", "This proxy only supports GET.")
		return
	}

	// Handle the GET proxy request
	handleGetProxy(conn, req)
}

// --- Proxy Logic ---

func handleGetProxy(conn net.Conn, req *http.Request) {

	// This is necessary for a robust proxy: handle relative paths from the client
	targetURL := req.URL.String()
	if !req.URL.IsAbs() {
		// If the URL is not absolute, we need to rebuild it using the Host header
		targetURL = "http://" + req.Host + req.URL.Path
		if req.URL.RawQuery != "" {
			targetURL += "?" + req.URL.RawQuery
		}
	}

	// 1. Create an outbound request
	outReq, err := http.NewRequest(req.Method, targetURL, nil)
	if err != nil {
		log.Printf("Error creating outbound request: %v", err)
		sendErrorResponse(conn, 400, "Bad Request", "Invalid target URL.")
		return
	}

	// 2. Copy the client's headers
	outReq.Header = req.Header

	// 3. Create an HTTP client with a timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 4. Send the request to the "origin server"
	resp, err := client.Do(outReq)
	if err != nil {
		log.Printf("Error fetching remote URL: %v", err)
		// 502 Bad Gateway is the standard error when a proxy can't reach the upstream server
		sendErrorResponse(conn, 502, "Bad Gateway", "The origin server could not be reached.")
		return
	}
	defer resp.Body.Close()

	// 5. --- Key: Write the "origin server's" response back to our "original client" ---

	// 5.1. Write the status line, directly use the origin server's 'Proto' and 'Status'
	if _, err := fmt.Fprintf(conn, "%s %s\r\n", resp.Proto, resp.Status); err != nil {
		log.Printf("Error writing status line to client: %v", err)
		return
	}

	// 5.2. Write all response headers
	for key, values := range resp.Header {
		for _, value := range values {
			if _, err := fmt.Fprintf(conn, "%s: %s\r\n", key, value); err != nil {
				log.Printf("Error writing header to client: %v", err)
				return
			}
		}
	}

	// 5.3. Write a blank line to separate headers and body
	if _, err := fmt.Fprintf(conn, "\r\n"); err != nil {
		log.Printf("Error writing CRLF to client: %v", err)
		return
	}

	// 5.4. Copy the response body (HTML, image, etc.) directly to the client
	if _, err := io.Copy(conn, resp.Body); err != nil {
		log.Printf("Error copying response body to client: %v", err)
	}
}

// --- Helper Function ---

// sendErrorResponse writes a simple HTTP error response to the client.
func sendErrorResponse(conn net.Conn, statusCode int, statusText, body string) {
	log.Printf("Sending error: %d %s", statusCode, statusText)

	bodyBytes := []byte(body + "\n")

	// Use HTTP/1.1 (more standard than 1.0)
	fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\n", statusCode, statusText)
	fmt.Fprintf(conn, "Content-Type: text/plain\r\n")
	fmt.Fprintf(conn, "Content-Length: %d\r\n", len(bodyBytes))
	fmt.Fprintf(conn, "\r\n")

	if _, err := conn.Write(bodyBytes); err != nil {
		log.Printf("Error sending error body: %v", err)
	}
}

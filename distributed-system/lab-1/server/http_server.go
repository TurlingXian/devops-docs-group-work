package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// --- Configuration ---

// FILE_ROOT specifies the directory to serve files from.
// All GETs and POSTs will operate relative to this directory.
const FILE_ROOT = "../client/webroot"

// MAX_CLIENTS defines the maximum number of concurrent connections (Requirement: 10).
const MAX_CLIENTS = 10

// mimeTypes maps file extensions to their corresponding Content-Type header.
var mimeTypes = map[string]string{
	".html": "text/html",
	".txt":  "text/plain",
	".gif":  "image/gif",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".css":  "text/css",
}

// --- Main Server ---

func main() {
	// 1. Check for command-line argument (Requirement: take port as argument)
	if len(os.Args) != 2 {
		fmt.Println("Usage: ./http_server <port>")
		os.Exit(1)
	}
	port := os.Args[1]

	// Ensure the webroot directory exists
	if err := os.MkdirAll(FILE_ROOT, 0755); err != nil {
		log.Fatalf("Failed to create webroot directory: %v", err)
	}
	log.Printf("Serving files from: %s", FILE_ROOT)

	// 2. Create the TCP listener (Requirement: Use 'net' package)
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}
	defer listener.Close()
	log.Printf("Server listening on port %s...", port)

	// 3. Create semaphore to limit concurrency (Requirement: at most 10)
	// This is a buffered channel. Writing to it "takes" a slot.
	// Reading from it "releases" a slot.
	semaphore := make(chan struct{}, MAX_CLIENTS)

	// 4. The main accept loop
	for {
		// Wait for and accept a new client connection
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue // Don't crash the server
		}

		// Requirement: Wait if 10 processes are already running
		semaphore <- struct{}{} // Acquire a slot. Blocks if channel is full.

		// Requirement: Spawn a new Go routine for each request
		go handleConnection(conn, semaphore)
	}
}

// --- Connection Handling ---

// handleConnection reads the request, routes it, and ensures cleanup.
func handleConnection(conn net.Conn, semaphore chan struct{}) {
	// Ensure connection is closed and semaphore slot is released when done
	defer conn.Close()
	defer func() {
		<-semaphore // Release the slot
	}()

	// Use bufio.Reader to read data from the connection
	reader := bufio.NewReader(conn)

	// Requirement: Use 'net/http' ONLY for parsing
	req, err := http.ReadRequest(reader)
	if err != nil {
		if err != io.EOF {
			log.Printf("Error reading request: %v", err)
			// Requirement: Invalid request -> 400
			sendErrorResponse(conn, 400, "Bad Request")
		}
		// EOF means client disconnected, just return
		return
	}

	log.Printf("Received: %s %s", req.Method, req.URL.Path)

	// Requirement: Route based on method (GET, POST, other)
	switch req.Method {
	case "GET":
		handleGet(conn, req)
	case "POST":
		handlePost(conn, req)
	default:
		// Requirement: Other methods -> 501
		sendErrorResponse(conn, 501, "Not Implemented")
	}
}

// --- Method Handlers ---

// handleGet serves a file if it exists and has a valid extension.
func handleGet(conn net.Conn, req *http.Request) {
	// Sanitize the file path to prevent directory traversal attacks
	// filepath.Clean removes ".."
	// The strings.HasPrefix check ensures the path is still within FILE_ROOT
	filePath := filepath.Join(FILE_ROOT, filepath.Clean(req.URL.Path))
	if !strings.HasPrefix(filePath, FILE_ROOT) {
		sendErrorResponse(conn, 400, "Bad Request")
		return
	}

	// Requirement: Check for valid extensions
	ext := filepath.Ext(filePath)
	mimeType, ok := mimeTypes[ext]
	if !ok {
		// Requirement: Other extensions -> 400
		sendErrorResponse(conn, 400, "Bad Request")
		return
	}

	// Try to open the file
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Requirement: File not found -> 404
			sendErrorResponse(conn, 404, "Not Found")
		} else {
			// Requirement: Other errors -> 400
			log.Printf("Error opening file %s: %v", filePath, err)
			sendErrorResponse(conn, 400, "Bad Request")
		}
		return
	}
	defer file.Close()

	// Get file info (for Content-Length)
	stat, err := file.Stat()
	if err != nil {
		log.Printf("Error getting file stats: %v", err)
		sendErrorResponse(conn, 400, "Bad Request")
		return
	}

	// Write the HTTP success response headers manually
	// This is the core part, not using http.Serve()
	fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\n")
	fmt.Fprintf(conn, "Content-Type: %s\r\n", mimeType)
	fmt.Fprintf(conn, "Content-Length: %d\r\n", stat.Size())
	fmt.Fprintf(conn, "\r\n") // End of headers

	// Stream the file content to the client
	if _, err := io.Copy(conn, file); err != nil {
		log.Printf("Error sending file body: %v", err)
	}
}

// handlePost saves the request body to a file.
func handlePost(conn net.Conn, req *http.Request) {
	// Requirement: Store files and make them accessible via GET
	// We'll use the same path logic as GET
	filePath := filepath.Join(FILE_ROOT, filepath.Clean(req.URL.Path))
	if !strings.HasPrefix(filePath, FILE_ROOT) {
		sendErrorResponse(conn, 400, "Bad Request")
		return
	}

	// Ensure the target directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Error creating directory %s: %v", dir, err)
		sendErrorResponse(conn, 400, "Bad Request") // Or 500, but 400 fits spec
		return
	}

	// Create the file (truncates if it exists)
	file, err := os.Create(filePath)
	if err != nil {
		log.Printf("Error creating file %s: %v", filePath, err)
		sendErrorResponse(conn, 400, "Bad Request")
		return
	}
	defer file.Close()

	// Copy the request body directly into the file
	if _, err := io.Copy(file, req.Body); err != nil {
		log.Printf("Error writing to file: %v", err)
		sendErrorResponse(conn, 400, "Bad Request")
		return
	}

	// Send a "201 Created" response
	log.Printf("File created: %s", filePath)
	fmt.Fprintf(conn, "HTTP/1.1 201 Created\r\n")
	fmt.Fprintf(conn, "Content-Length: 0\r\n")
	fmt.Fprintf(conn, "\r\n")
}

// --- Helper Function ---

// sendErrorResponse writes a simple HTTP error response to the client.
func sendErrorResponse(conn net.Conn, statusCode int, statusText string) {
	log.Printf("Sending error: %d %s", statusCode, statusText)
	fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\n", statusCode, statusText)
	fmt.Fprintf(conn, "Content-Length: 0\r\n")
	fmt.Fprintf(conn, "\r\n")
}

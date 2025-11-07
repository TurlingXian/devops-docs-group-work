package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMain sets up our test environment (e.g., creating files to GET)
func TestMain(m *testing.M) {
	// Create a dummy 'static' directory
	if err := os.MkdirAll("static", 0755); err != nil {
		log.Fatalf("Could not create static dir: %v", err)
	}
	// Create a dummy 'uploads' directory
	if err := os.MkdirAll("uploads", 0755); err != nil {
		log.Fatalf("Could not create uploads dir: %v", err)
	}
	// Create a dummy file to serve
	hello := []byte("<html>Hello World</html>")
	if err := os.WriteFile("static/index.html", hello, 0644); err != nil {
		log.Fatalf("Could not create test file: %v", err)
	}

	// Run all tests
	code := m.Run()

	// Clean up
	os.RemoveAll("static")
	os.RemoveAll("uploads")
	os.Exit(code)
}

// Helper function to start a server and return its address
func startTestServer(t *testing.T) (addr string, stop func()) {
	s, err := newServer(":0") //
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	s.Start() //

	// Get the dynamic address
	addr = s.listener.Addr().String()

	stop = func() {
		s.Stop() //
	}
	return addr, stop
}

// Helper to make a raw HTTP request (since we're not using http.Client)
func sendRequest(t *testing.T, addr, requestStr string) string {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	if _, err := fmt.Fprint(conn, requestStr); err != nil {
		t.Fatalf("Failed to write request: %v", err)
	}

	respBytes, err := io.ReadAll(conn)
	if err != nil && err != io.EOF {
		t.Fatalf("Failed to read response: %v", err)
	}
	return string(respBytes)
}

// TestGET_OK checks for a valid 200 OK file
func TestGET_OK(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	req := "GET /index.html HTTP/1.1\r\nHost: test\r\n\r\n"
	resp := sendRequest(t, addr, req)

	if !strings.Contains(resp, "HTTP/1.1 200 OK") {
		t.Fatalf("Expected 200 OK, got: %s", resp)
	}
	if !strings.Contains(resp, "Hello World") {
		t.Fatalf("Expected 'Hello World' in body, got: %s", resp)
	}
	if !strings.Contains(resp, "Content-Type: text/html") {
		t.Fatalf("Expected Content-Type text/html, got: %s", resp)
	}
}

// TestGET_NotFound checks for a 404
func TestGET_NotFound(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	req := "GET /nonexistent.txt HTTP/1.1\r\nHost: test\r\n\r\n"
	resp := sendRequest(t, addr, req)

	if !strings.Contains(resp, "HTTP/1.1 404 Not Found") {
		t.Fatalf("Expected 404 Not Found, got: %s", resp)
	}
}

// TestGET_BadRequest checks for an unsupported extension (400)
func TestGET_BadRequest(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	req := "GET /image.zip HTTP/1.1\r\nHost: test\r\n\r\n"
	resp := sendRequest(t, addr, req)

	if !strings.Contains(resp, "HTTP/1.1 400 Bad Request") {
		t.Fatalf("Expected 400 Bad Request, got: %s", resp)
	}
}

// TestPOST_Valid checks for a 201 Created
func TestPOST_Valid(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	body := "This is a test upload."
	req := fmt.Sprintf(
		"POST /test.txt HTTP/1.1\r\nHost: test\r\nContent-Length: %d\r\n\r\n%s",
		len(body), body,
	)
	resp := sendRequest(t, addr, req)

	if !strings.Contains(resp, "HTTP/1.1 201 Created") {
		t.Fatalf("Expected 201 Created, got: %s", resp)
	}
	if !strings.Contains(resp, "Location: /test.txt") {
		t.Fatalf("Expected Location header, got: %s", resp)
	}

	// Check if file was actually created
	if _, err := os.Stat("uploads/test.txt"); os.IsNotExist(err) {
		t.Fatal("POST success, but file was not created on disk")
	}
}

// TestConcurrencyLimit checks that the MAX_CONNECTION limit works
func TestConcurrencyLimit(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	numClients := 15 // More than MAX_CONNECTION

	var wg sync.WaitGroup
	wg.Add(numClients)

	startTime := time.Now()

	for i := 0; i < numClients; i++ {
		go func() {
			defer wg.Done()
			req := "GET /index.html HTTP/1.1\r\nHost: test\r\n\r\n"
			// This will block until the server can handle it
			sendRequest(t, addr, req)
		}()
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	// --- Verification ---
	// 15 clients * 1s work = 15 "work-seconds"
	// With 10 workers, it must take at least 2 batches.
	// (10 clients @ 1s) + (5 clients @ 1s) = 2 seconds minimum.
	minExpectedTime := 2 * time.Second

	if totalDuration < minExpectedTime {
		t.Errorf("Connection limit was NOT respected! "+
			"Test finished in %v (too fast), expected at least %v",
			totalDuration, minExpectedTime)
	}

	log.Printf("Concurrency test passed: %d clients served in %v (expected > %v)",
		numClients, totalDuration, minExpectedTime)
}

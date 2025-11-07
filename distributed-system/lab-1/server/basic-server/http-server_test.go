package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest" // We need this to create a fake backend server
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestServerAsProxy checks both proxy functionality AND the connection limit.
func TestServerAsProxy(t *testing.T) {

	// 1. --- Create a Fake Backend Server ---
	// This is the "real" web server our proxy will talk to.
	// It's set to be slow (1 second) to help test concurrency.
	const backendWorkTime = 1 * time.Second

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(backendWorkTime) // Simulate slow work
		fmt.Fprintln(w, "Hello from the backend!")
	}))
	defer backend.Close()

	// 2. --- Start Your Proxy Server (Server Under Test) ---
	s, err := newServer(":0") // Listen on a random port
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	// We MUST change the worker to call the proxy handler.
	// This test ASSUMES you've made this change in http-server.go:
	// func (s *server) worker() {
	// 	  ...
	//    for conn := range s.connection {
	//        s.proxyHandleConnection(conn) // <-- MUST BE THIS
	//    }
	// }
	s.Start()
	defer s.Stop()

	proxyAddr := s.listener.Addr().String()
	log.Printf("Test Proxy Server started on %s", proxyAddr)
	log.Printf("Test Backend Server started on %s", backend.URL)

	// 3. --- Run The Test ---
	const numClients = 15 // More than MAX_CONNECTION (10)

	var wg sync.WaitGroup
	wg.Add(numClients)

	var totalServed atomic.Int32
	var totalErrors atomic.Int32

	// We'll use the backend's URL to tell the proxy where to go
	backendURL, _ := url.Parse(backend.URL)

	log.Printf("Launching %d clients...", numClients)
	startTime := time.Now()

	for i := 0; i < numClients; i++ {
		go func(clientID int) {
			defer wg.Done()

			// Connect to the PROXY, not the backend
			conn, err := net.Dial("tcp", proxyAddr)
			if err != nil {
				t.Errorf("Client %d failed to dial proxy: %v", clientID, err)
				totalErrors.Add(1)
				return
			}
			defer conn.Close()

			// --- This is the fix ---
			// We act like a real client: WE SPEAK FIRST.
			// We send a valid HTTP request to the proxy, asking
			// for the backend server's content.
			reqStr := fmt.Sprintf("GET /testpath HTTP/1.1\r\nHost: %s\r\n\r\n", backendURL.Host)

			if _, err := fmt.Fprint(conn, reqStr); err != nil {
				t.Errorf("Client %d failed to write request: %v", clientID, err)
				totalErrors.Add(1)
				return
			}
			// --- End fix ---

			// Now we wait for the full response from the proxy
			respBytes, err := io.ReadAll(conn)
			if err != nil && err != io.EOF {
				t.Errorf("Client %d read error: %v", clientID, err)
				totalErrors.Add(1)
				return
			}

			// Check if the proxying worked
			if !strings.Contains(string(respBytes), "Hello from the backend!") {
				t.Errorf("Client %d: invalid response body: %s", clientID, string(respBytes))
				totalErrors.Add(1)
				return
			}

			// We were served correctly
			totalServed.Add(1)
		}(i)
	}

	wg.Wait()
	totalDuration := time.Since(startTime)
	log.Printf("All clients finished in: %v", totalDuration)

	// 4. --- Verify the Results ---
	if totalErrors.Load() > 0 {
		t.Errorf("Test failed: %d clients experienced errors", totalErrors.Load())
	}

	if totalServed.Load() != numClients {
		t.Errorf("Not all clients were served! Expected %d, got %d",
			numClients, totalServed.Load())
	}

	// **This is the main concurrency check:**
	// 15 clients * 1s work = 15 "work-seconds"
	// With 10 workers, it must take at least 2 batches.
	// (10 clients @ 1s) + (5 clients @ 1s) = 2 seconds minimum.
	minExpectedTime := 2 * backendWorkTime

	if totalDuration < minExpectedTime {
		t.Errorf("Connection limit was NOT respected! "+
			"Test finished in %v (too fast), expected at least %v",
			totalDuration, minExpectedTime)
	}

	// Check if it was reasonable (not 10 seconds)
	// (Add 1s for connection overhead)
	maxExpectedTime := (2 * backendWorkTime) + (1 * time.Second)
	if totalDuration > maxExpectedTime {
		t.Errorf("Test took too long: %v, expected less than %v",
			totalDuration, maxExpectedTime)
	}

	log.Printf("Test complete. %d clients served.", totalServed.Load())
	log.Printf("Concurrency test passed: total time %v (expected ~%v)",
		totalDuration, minExpectedTime)
}

package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

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

func startTestServer(t *testing.T, isProxy bool) (addr string, stop func()) {
	s, err := newServer(":0") //
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	// Start in the requested mode
	s.Start(isProxy)

	// Get the dynamic address
	addr = s.listener.Addr().String()

	stop = func() {
		s.Stop()
	}
	return addr, stop
}

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

func TestWebServerMode(t *testing.T) {
	addr, stop := startTestServer(t, false)
	defer stop()

	t.Run("GET_OK", func(t *testing.T) {
		req := "GET /index.html HTTP/1.1\r\nHost: test\r\n\r\n"
		resp := sendRequest(t, addr, req)

		if !strings.Contains(resp, "HTTP/1.1 200 OK") {
			t.Fatalf("Expected 200 OK, got: %s", resp)
		}
		if !strings.Contains(resp, "Hello World") {
			t.Fatalf("Expected 'Hello World' in body, got: %s", resp)
		}
	})

	t.Run("GET_NotFound", func(t *testing.T) {
		req := "GET /nonexistent.txt HTTP/1.1\r\nHost: test\r\n\r\n"
		resp := sendRequest(t, addr, req)

		if !strings.Contains(resp, "HTTP/1.1 404 Not Found") {
			t.Fatalf("Expected 404 Not Found, got: %s", resp)
		}
	})

	t.Run("GET_BadRequest", func(t *testing.T) {
		req := "GET /image.zip HTTP/1.1\r\nHost: test\r\n\r\n"
		resp := sendRequest(t, addr, req)

		if !strings.Contains(resp, "HTTP/1.1 400 Bad Request") {
			t.Fatalf("Expected 400 Bad Request, got: %s", resp)
		}
	})

	t.Run("POST_Valid", func(t *testing.T) {
		body := "This is a test upload."
		req := fmt.Sprintf(
			"POST /test.txt HTTP/1.1\r\nHost: test\r\nContent-Length: %d\r\n\r\n%s",
			len(body), body,
		)
		resp := sendRequest(t, addr, req)

		if !strings.Contains(resp, "HTTP/1.1 201 Created") {
			t.Fatalf("Expected 201 Created, got: %s", resp)
		}
		if _, err := os.Stat("uploads/test.txt"); os.IsNotExist(err) {
			t.Fatal("POST success, but file was not created on disk")
		}
	})

	t.Run("ConcurrencyLimit", func(t *testing.T) {
		numClients := 15 // Larger than max connections
		var wg sync.WaitGroup
		wg.Add(numClients)
		startTime := time.Now()

		for i := 0; i < numClients; i++ {
			go func() {
				defer wg.Done()
				req := "GET /index.html HTTP/1.1\r\nHost: test\r\n\r\n"
				sendRequest(t, addr, req)
			}()
		}
		wg.Wait()
		totalDuration := time.Since(startTime)
		minExpectedTime := 2 * time.Second // 15 clients * 1s work / 10 workers

		if totalDuration < minExpectedTime {
			t.Errorf("Connection limit was NOT respected! "+
				"Test finished in %v, expected at least %v",
				totalDuration, minExpectedTime)
		}
	})
}

func TestProxyMode(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			time.Sleep(1 * time.Second)
			fmt.Fprintln(w, "Hello from the backend!")
		}
	}))
	defer backend.Close()
	backendURL, _ := url.Parse(backend.URL)

	proxyAddr, stop := startTestServer(t, true)
	defer stop()

	t.Run("ProxyGET_OK", func(t *testing.T) {
		req := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", backendURL.Host)
		resp := sendRequest(t, proxyAddr, req)

		if !strings.Contains(resp, "HTTP/1.1 200 OK") {
			t.Fatalf("Expected 200 OK, got: %s", resp)
		}
		if !strings.Contains(resp, "Hello from the backend!") {
			t.Fatalf("Expected 'Hello from the backend!', got: %s", resp)
		}
	})

	t.Run("ProxyPOST_NotImplemented", func(t *testing.T) {
		req := fmt.Sprintf("POST / HTTP/1.1\r\nHost: %s\r\n\r\n", backendURL.Host)
		resp := sendRequest(t, proxyAddr, req)

		if !strings.Contains(resp, "HTTP/1.1 501 Not Implemented") {
			t.Fatalf("Expected 501 Not Implemented, got: %s", resp)
		}
	})

	t.Run("ProxyConcurrencyLimit", func(t *testing.T) {
		numClients := 15
		var wg sync.WaitGroup
		wg.Add(numClients)
		startTime := time.Now()

		for i := 0; i < numClients; i++ {
			go func() {
				defer wg.Done()
				req := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", backendURL.Host)
				sendRequest(t, proxyAddr, req)
			}()
		}
		wg.Wait()
		totalDuration := time.Since(startTime)
		minExpectedTime := 2 * time.Second

		if totalDuration < minExpectedTime {
			t.Errorf("Proxy connection limit was NOT respected! "+
				"Test finished in %v, expected at least %v",
				totalDuration, minExpectedTime)
		}
	})
}

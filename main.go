package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const socketDir = "/run/nofan"

var socketPath = filepath.Join(socketDir, "nofan.sock")

type Request struct {
	Cmd string `json:"cmd"`
}

type Response struct {
	Status  string `json:"status"`
	Counter int64  `json:"counter,omitempty"`
	Error   string `json:"error,omitempty"`
}

var (
	counter int64
	paused  bool
	mu      sync.RWMutex
)

func runServer() {
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create socket: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

  if err := os.Chmod(socketPath, 0660); err != nil {
    fmt.Fprintf(os.Stderr, "Failed to set socket permissions: %v\n", err)
  }

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			mu.RLock()
			p := paused
			mu.RUnlock()

			if !p {
				mu.Lock()
				counter++
				mu.Unlock()
			}
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-sigChan:
					return
				default:
					fmt.Fprintf(os.Stderr, "Accept error: %v\n", err)
				}
				return
			}
			go handleConnection(conn)
		}
	}()

	<-sigChan
	fmt.Println("Shutting down...")
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	data, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}

	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		sendResponse(conn, Response{Status: "error", Error: "Invalid JSON"})
		return
	}

	switch req.Cmd {
	case "status":
		mu.RLock()
		c := counter
		p := paused
		mu.RUnlock()
		sendResponse(conn, Response{
			Status:  "ok",
			Counter: c,
			Error:   fmt.Sprintf("paused:%t", p),
		})
	case "pause":
		mu.Lock()
		paused = true
		mu.Unlock()
		sendResponse(conn, Response{Status: "ok"})
	case "resume":
		mu.Lock()
		paused = false
		mu.Unlock()
		sendResponse(conn, Response{Status: "ok"})
	default:
		sendResponse(conn, Response{Status: "error", Error: "Unknown command"})
	}
}

func sendResponse(conn net.Conn, resp Response) {
	data, _ := json.Marshal(resp)
	conn.Write(append(data, '\n'))
}

func runClient() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s client <command>\n", os.Args[0])
		os.Exit(1)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to server: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	req := Request{Cmd: os.Args[2]}
	data, _ := json.Marshal(req)
	conn.Write(append(data, '\n'))

	reader := bufio.NewReader(conn)
	response, err := reader.ReadBytes('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read response: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(string(response))
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		runServer()
	} else {
		runClient()
	}
}

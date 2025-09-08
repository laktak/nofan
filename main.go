package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	// "os/signal"
	// "syscall"
)

var (
	socketDir  string
	socketPath string
)

type Request struct {
	Cmd string `json:"cmd"`
}

type Response struct {
	Status   string  `json:"status"`
	FanSpeed int64   `json:"fanSpeed,omitempty"`
	CpuTemp  float64 `json:"cpuTemp,omitempty"`
	Error    string  `json:"error,omitempty"`
}

type Server struct {
	counter int64
	cpuTemp float64
	paused  bool
	mu      sync.RWMutex
}

func NewServer() *Server {
	return &Server{
		counter: 0,
		paused:  false,
	}
}

func (s *Server) run() {
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
		for {
			conn, err := listener.Accept()
			if err != nil {
				continue
			}
			go s.handleConnection(conn)
		}
	}()

	for {
		select {
		case <-ticker.C:
			t := getCpuTemp()
			s.mu.RLock()
			s.cpuTemp = t
			if !s.paused {
				s.counter++
				fmt.Printf("CPU Temperature: %.2f°C\n", t)
				fmt.Printf("Counter: %d\n", s.counter)
			}
			s.mu.RUnlock()
		}
	}
}

func (s *Server) handleConnection(conn net.Conn) {
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
		s.mu.RLock()
		t := s.cpuTemp
		p := s.paused
		s.mu.RUnlock()
		sendResponse(conn, Response{
			Status:  "ok",
			CpuTemp: t,
			Error:   fmt.Sprintf("paused:%t", p),
		})
	case "pause":
		s.mu.Lock()
		s.paused = true
		s.mu.Unlock()
		sendResponse(conn, Response{Status: "ok"})
	case "resume":
		s.mu.Lock()
		s.paused = false
		s.mu.Unlock()
		sendResponse(conn, Response{Status: "ok"})
	default:
		sendResponse(conn, Response{Status: "error", Error: "Unknown command"})
	}
}

func sendResponse(conn net.Conn, resp Response) {
	data, _ := json.Marshal(resp)
	conn.Write(append(data, '\n'))
}

func getCpuTemp() float64 {
	data, err := ioutil.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		fmt.Printf("Error reading temperature: %v\n", err)
		return 0
	}

	tempStr := strings.TrimSpace(string(data))
	temp, err := strconv.Atoi(tempStr)
	if err != nil {
		fmt.Printf("Error converting temperature: %v\n", err)
		return 0
	}
	return float64(temp) / 1000.0
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
	socketDir = os.Getenv("NOFAN_SOCKET_DIR")
	if socketDir == "" {
		socketDir = "/run/nofan"
	}

	socketPath = filepath.Join(socketDir, "nofan.sock")

	if len(os.Args) > 1 && os.Args[1] == "run" {
		NewServer().run()
	} else {
		runClient()
	}
}

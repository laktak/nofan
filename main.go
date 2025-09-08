package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	cpuMaxHist      = 20
	cpuHistPrio     = 5
	switchThreshold = 2
)

var (
	socketDir  string
	socketPath string
)

type CurveEntry struct {
	temp  float64
	speed int
}

var curve = []CurveEntry{
	CurveEntry{51, 15},
	CurveEntry{55, 20},
	CurveEntry{60, 30},
	CurveEntry{80, 90},
	CurveEntry{90, 100},
}

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
	cpuTemp     float64
	cpuTempHist []float64
	paused      bool
	mu          sync.RWMutex
}

func NewServer() *Server {
	return &Server{
		paused:      false,
		cpuTempHist: make([]float64, 0),
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
			cpuTemp := getCpuTemp()
			usage := getCpuUsage()
			s.mu.Lock()
			s.cpuTemp = cpuTemp
			for len(s.cpuTempHist) < cpuMaxHist+1 {
				s.cpuTempHist = append(s.cpuTempHist, cpuTemp)
			}
			s.cpuTempHist = s.cpuTempHist[1 : cpuMaxHist+1]
			var t1, t2 float64 = 0.0, 0.0
			for i := 0; i < cpuMaxHist-cpuHistPrio; i++ {
				t1 += s.cpuTempHist[i]
			}
			for i := cpuMaxHist - cpuHistPrio; i < cpuMaxHist; i++ {
				t2 += s.cpuTempHist[i]
			}
			avg1 := t1 / (cpuMaxHist - cpuHistPrio)
			avg2 := t2 / (cpuHistPrio)
			waTemp := max(avg1, avg2)

			fan := 0
			// next := 100
			fTemp := 0.0
			for _, e := range curve {
				if waTemp > e.temp {
					fTemp = e.temp
					fan = e.speed
				} else {
					// adjust speed if necessary
					if fan > 0 {
						adjust := (waTemp - fTemp) / (e.temp - fTemp)
						next := float64(e.speed-fan) * adjust
						fmt.Printf("- %.2f of %d: %.1f\n", adjust, e.speed-fan, next)
						fan += int(next)
					}
					break
				}
			}

			if !s.paused {
				fmt.Printf("Temp: %.2f°C (%.2f%%) - fan %d\n", waTemp, usage*100.0, fan)
				// fmt.Printf("%v\n", s.cpuTempHist)
			}
			s.mu.Unlock()
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

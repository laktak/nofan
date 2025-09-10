package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	cpuMaxHist     = 20
	cpuHistPrio    = 5
	updateInterval = 4
	tempThreshold  = 2.5
	speedThreshold = 4
	minDownTick    = updateInterval * 4
)

var (
	dryRun     = false
	socketDir  string
	socketPath string
)

var log *Logger

type Request struct {
	Cmd string `json:"cmd"`
}

type Response struct {
	FanSpeed int     `json:"fanSpeed,omitempty"`
	CpuTemp  float64 `json:"cpuTemp,omitempty"`
	Error    string  `json:"error,omitempty"`
}

type Server struct {
	spec        Spec
	cpuTemp     float64
	cpuTempHist []float64
	fanSpeed    int
	slot        *SlotEntry
	slotTick    int64
	ticks       int64
	mu          sync.RWMutex
	paused      bool
}

func NewServer(spec Spec) *Server {
	return &Server{
		spec:        spec,
		paused:      false,
		cpuTempHist: make([]float64, 0),
		fanSpeed:    -speedThreshold - 1,
	}
}

func (s *Server) pushCpuTemp(temp float64) {
	s.cpuTemp = temp
	for len(s.cpuTempHist) < cpuMaxHist+1 {
		s.cpuTempHist = append(s.cpuTempHist, temp)
	}
	s.cpuTempHist = s.cpuTempHist[1 : cpuMaxHist+1]
}

func (s *Server) getWeightedCpuTemp() float64 {
	var t1, t2 float64 = 0.0, 0.0
	for i := 0; i < cpuMaxHist-cpuHistPrio; i++ {
		t1 += s.cpuTempHist[i]
	}
	for i := cpuMaxHist - cpuHistPrio; i < cpuMaxHist; i++ {
		t2 += s.cpuTempHist[i]
	}
	avg1 := t1 / (cpuMaxHist - cpuHistPrio)
	avg2 := t2 / (cpuHistPrio)
	return max(avg1, avg2)
}

func (s *Server) run() {
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Error("failed to create socket: %v", err)
		os.Exit(1)
	}
	defer listener.Close()
	defer os.Remove(socketPath)

	if err := os.Chmod(socketPath, 0660); err != nil {
		log.Error("failed to set socket permissions: %v", err)
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
			// cpuUsage := getCpuUsage()
			s.mu.Lock()
			s.pushCpuTemp(cpuTemp)

			if s.ticks%updateInterval == 0 {

				wtemp := s.getWeightedCpuTemp()
				if s.slot == nil || !s.slot.isInSlot(wtemp, tempThreshold) {
					ns := s.spec.find(wtemp)
					if s.slot == nil || ns.from > s.slot.from || s.ticks-s.slotTick >= minDownTick {
						s.slot = s.spec.find(wtemp)
						s.slotTick = s.ticks
						log.Info("- set slot: %.0f", s.slot.from)
					}
				} else if s.slot.isInSlot(wtemp, 0) {
					s.slotTick = s.ticks
				}

				fan := s.slot.speed
				if abs(s.fanSpeed-fan) > speedThreshold || fan == 100 && s.fanSpeed < 100 {
					s.fanSpeed = fan
					log.Info("- set speed: %d", fan)
					if !dryRun {
						if _, err := exec.Command("ectool", "fanduty", strconv.Itoa(fan)).Output(); err != nil {
							log.Error("failed to set fan speed: %v", err)
						}
					}
				}

				log.Info("[%.0f-%.0f:%d] %.2f°C - fan %d (%d)", s.slot.from, s.slot.to, s.slot.speed, wtemp, s.fanSpeed, fan)
				log.Debug("%v", s.cpuTempHist)
			}

			s.ticks++
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
		sendResponse(conn, Response{Error: "Invalid JSON"})
		return
	}

	switch req.Cmd {
	case "status":
		s.mu.RLock()
		t := s.cpuTemp
		s.mu.RUnlock()
		sendResponse(conn, Response{
			CpuTemp: t,
		})
	case "pause":
		s.mu.Lock()
		s.paused = true
		s.mu.Unlock()
		sendResponse(conn, Response{})
	case "resume":
		s.mu.Lock()
		s.paused = false
		s.mu.Unlock()
		sendResponse(conn, Response{})
	default:
		sendResponse(conn, Response{Error: "Unknown command"})
	}
}

func abs(n int) int { return max(n, -n) }

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

	dryRun = os.Getenv("NOFAN_DRYRUN") != ""
	if dryRun {
		fmt.Fprintf(os.Stderr, "dry-run!\n")
	}
	socketDir = os.Getenv("NOFAN_SOCKET_DIR")
	if socketDir == "" {
		socketDir = "/run/nofan"
	}

	socketPath = filepath.Join(socketDir, "nofan.sock")

	if len(os.Args) > 1 && os.Args[1] == "run" {

		var err error
		log, err = NewLogger("/tmp/nofan.log")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to open log: %v\n", err)
			os.Exit(1)
		}
		defer log.Close()

		NewServer(NewSpec(config)).run()
	} else {
		runClient()
	}
}

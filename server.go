package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"os/exec"
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
	FanSpeed *int     `json:"fanSpeed,omitempty"`
	CpuTemp  *float64 `json:"cpuTemp,omitempty"`
	Error    string   `json:"error,omitempty"`
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

func (s *Server) setSpeed(fan int) {
	if _, err := exec.Command("ectool", "fanduty", strconv.Itoa(fan)).Output(); err != nil {
		log.Error("failed to set fan speed: %v", err)
	}
}

func (s *Server) setSpeedAuto() {
	if _, err := exec.Command("ectool", "autofanctrl").Output(); err != nil {
		log.Error("failed to set fan to auto: %v", err)
	}
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

			if s.ticks%updateInterval == 0 && !s.paused {

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
				if s.fanSpeed < 0 || abs(s.fanSpeed-fan) > speedThreshold || fan == 100 && s.fanSpeed < 100 {
					s.fanSpeed = fan
					log.Info("- set speed: %d", fan)
					if !dryRun {
						s.setSpeed(fan)
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
		sendResponse(conn, Response{Error: "invalid json"})
		return
	}

	switch req.Cmd {
	case "status":
		s.mu.RLock()
		t := s.cpuTemp
		f := s.fanSpeed
		s.mu.RUnlock()
		sendResponse(conn, Response{
			CpuTemp:  &t,
			FanSpeed: &f,
		})
	case "pause":
		s.mu.Lock()
		s.fanSpeed = -1
		s.paused = true
		s.mu.Unlock()
		log.Info("pause")
		s.setSpeedAuto()
		sendResponse(conn, Response{})
	case "resume":
		s.mu.Lock()
		s.paused = false
		s.mu.Unlock()
		log.Info("resume")
		sendResponse(conn, Response{})
	default:
		sendResponse(conn, Response{Error: "unknown command"})
	}
}

func abs(n int) int { return max(n, -n) }

func sendResponse(conn net.Conn, resp Response) {
	data, _ := json.Marshal(resp)
	conn.Write(append(data, '\n'))
}


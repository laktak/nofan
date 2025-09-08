package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

var (
	lastCpuNonidle uint64 = 0
	lastCpuIdle    uint64 = 0
)

func getCpuTemp() float64 {
	// data, err := ioutil.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	// if err != nil {
	// 	fmt.Printf("Error reading temperature: %v\n", err)
	// 	return 0
	// }
	//
	// tempStr := strings.TrimSpace(string(data))
	// temp, err := strconv.Atoi(tempStr)
	// if err != nil {
	// 	fmt.Printf("Error converting temperature: %v\n", err)
	// 	return 0
	// }

	thermalDir := "/sys/class/thermal"
	entries, err := ioutil.ReadDir(thermalDir)
	if err != nil {
		fmt.Printf("Error reading thermal directory: %v\n", err)
		return 0
	}

	maxTemp := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "thermal_zone") {
			tempPath := thermalDir + "/" + entry.Name() + "/temp"
			if _, err := os.Stat(tempPath); err == nil {
				data, err := ioutil.ReadFile(tempPath)
				if err != nil {
					continue
				}

				tempStr := strings.TrimSpace(string(data))
				temp, err := strconv.Atoi(tempStr)
				if err != nil {
					continue
				}

				if temp > maxTemp {
					maxTemp = temp
				}
			}
		}
	}

	return float64(maxTemp) / 1000.0
}

func getCpuUsage() float64 {
	entries, err := ioutil.ReadFile("/proc/stat")
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(entries), "\n") {
		fields := strings.Fields(line)
		if fields[0] == "cpu" {
			var idle, nonidle uint64 = 0, 0
			n := len(fields)
			// label, user, nice, system, idle, iowait, irq, softirq, steal
			for i := 1; i < n; i++ {
				v, err := strconv.ParseUint(fields[i], 10, 64)
				if err != nil {
					// fmt.Println("Error: ", i, fields[i], err)
					return -1
				}
				if i == 4 || i == 5 {
					idle += v // idle + iowait
				} else {
					nonidle += v
				}
			}
			idle2 := idle - lastCpuIdle
			nonidle2 := nonidle - lastCpuNonidle
			lastCpuIdle = idle
			lastCpuNonidle = nonidle
			return float64(nonidle2) / float64(nonidle2+idle2)
		}
	}
	return -1
}

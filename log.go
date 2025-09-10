package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type Logger struct {
	file    *os.File
	log_con bool
	mu      sync.Mutex
}

func NewLogger(logFile string) (*Logger, error) {

	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	return &Logger{
		file:    file,
		log_con: os.Getenv("LOG_CON") != "",
	}, nil
}

func (l *Logger) Close() error {
	return l.file.Close()
}

func (l *Logger) log(level, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf("[%s] %s: %s\n", timestamp, level, fmt.Sprintf(format, args...))

	l.file.WriteString(message)
	l.file.Sync()

	if l.log_con {
		fmt.Fprint(os.Stderr, message)
	}
}

func (l *Logger) Debug(format string, args ...interface{}) {
	l.log("DBG", format, args...)
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.log("INF", format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.log("WRN", format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.log("ERR", format, args...)
}

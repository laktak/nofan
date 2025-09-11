package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
)


func runClient(args []string) {
	if len(args) < 2 {
		fmt.Println("usage: nofan <command>")
		os.Exit(1)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to server: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	req := Request{Cmd: args[1]}
	data, _ := json.Marshal(req)
	conn.Write(append(data, '\n'))

	reader := bufio.NewReader(conn)
	response, err := reader.ReadBytes('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read response: %v\n", err)
		os.Exit(1)
	}

	var res Response
	if err := json.Unmarshal(response, &res); err == nil {
		pretty, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(pretty))
	} else {
		fmt.Print(string(response))
	}
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
		runClient(os.Args)
	}
}

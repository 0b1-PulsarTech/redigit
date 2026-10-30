package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(64)
	}

	logCall()

	switch os.Args[1] {
	case "version":
		fmt.Println("go version fakego")
	case "env":
		report := map[string]string{}

		for _, name := range os.Args[3:] {
			value, isSet := os.LookupEnv("FAKEGO_" + name)
			if !isSet {
				value = os.Getenv(name)
			}

			report[name] = value
		}

		_ = json.NewEncoder(os.Stdout).Encode(report)
	}
}

func logCall() {
	path := os.Getenv("FAKEGO_LOG")
	if path == "" {
		return
	}

	cwd, _ := os.Getwd()

	handle, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}

	fmt.Fprintf(handle, "%s cwd=%s\n", os.Args[1], cwd)
	handle.Close()
}

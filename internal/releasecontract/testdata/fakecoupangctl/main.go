package main

import (
	"encoding/json"
	"os"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "version" {
		os.Exit(2)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{
		"name":    "coupangctl",
		"version": "1.2.3",
	})
}

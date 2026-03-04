package main

import (
	"fmt"
	"os"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("symphony %s (built %s)\n", version, buildTime)
		os.Exit(0)
	}
	fmt.Println("symphony: not yet implemented")
	os.Exit(1)
}

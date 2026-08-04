package main

import (
	"log"
	"os"

	"route-optimizer-go/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		log.Printf("error: %v", err)
		os.Exit(1)
	}
}

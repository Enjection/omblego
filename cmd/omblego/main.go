package main

import (
	"os"

	"omblego/cmd/omblego/commands"

	// Import device drivers to register them
	_ "omblego/internal/device/devices"
)

func main() {
	if err := commands.Execute(); err != nil {
		os.Exit(1)
	}
}

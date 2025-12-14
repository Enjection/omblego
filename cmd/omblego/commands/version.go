package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("omblego %s\n", Version)
		fmt.Println("Cross-platform Omron BLE blood pressure monitor reader")
	},
}

func init() {
	AddCommand(versionCmd)
}

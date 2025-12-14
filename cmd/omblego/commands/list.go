package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"omblego/internal/device"
)

var listCmd = &cobra.Command{
	Use:   "list-devices",
	Short: "List all supported Omron devices",
	Long:  `Lists all Omron blood pressure monitor models that omblego can communicate with.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Supported Omron devices:")
		for _, name := range device.ListDevices() {
			fmt.Printf("  - %s\n", name)
		}
	},
}

func init() {
	AddCommand(listCmd)
}

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Skiff version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("skiff v" + version)
		},
	}
}

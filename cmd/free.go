package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/Viljoen13/port-explorer/internal/ports"
	"github.com/spf13/cobra"
)

var flagFreeCount int

var freeCmd = &cobra.Command{
	Use:   "free [start]",
	Short: "Find a free port (default: first free port from 3000)",
	Long: `Print the first free TCP port at or above START (default 3000). A port counts
as free only when nothing is bound to it in any state and it can actually be
bound right now. Perfect for scripts:

  PORT=$(port-explorer free 8000) && my-server --port "$PORT"`,
	Example: `  port-explorer free
  port-explorer free 8000
  port-explorer free 3000 -n 3`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		start := uint16(3000)
		if len(args) == 1 {
			n, err := strconv.ParseUint(args[0], 10, 16)
			if err != nil || n == 0 {
				return fmt.Errorf("invalid start port %q", args[0])
			}
			start = uint16(n)
		}
		if flagFreeCount < 1 {
			flagFreeCount = 1
		}
		inUse := map[uint16]bool{}
		if entries, err := ports.List(); err == nil {
			for _, e := range entries {
				inUse[e.Port] = true
			}
		}
		found, err := ports.FindFree(start, flagFreeCount, inUse)
		for _, p := range found {
			fmt.Fprintln(os.Stdout, p)
		}
		return err
	},
}

func init() {
	freeCmd.Flags().IntVarP(&flagFreeCount, "count", "n", 1, "how many free ports to print")
	rootCmd.AddCommand(freeCmd)
}

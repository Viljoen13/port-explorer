package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/Viljoen13/port-explorer/internal/ports"
	"github.com/spf13/cobra"
)

var (
	flagForce bool
	flagYes   bool
)

var killCmd = &cobra.Command{
	Use:   "kill <port>",
	Short: "Kill the process running on a given port",
	Args:  cobra.ExactArgs(1),
	RunE:  runKill,
}

func init() {
	killCmd.Flags().BoolVarP(&flagForce, "force", "f", false, "send SIGKILL instead of SIGTERM")
	killCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "skip confirmation prompt")
	rootCmd.AddCommand(killCmd)
}

func runKill(cmd *cobra.Command, args []string) error {
	port, err := strconv.ParseUint(args[0], 10, 16)
	if err != nil {
		return fmt.Errorf("invalid port: %s", args[0])
	}

	entries, err := ports.List()
	if err != nil {
		return fmt.Errorf("listing ports: %w", err)
	}

	// Find processes on this port
	var matches []ports.PortInfo
	for _, e := range entries {
		if uint64(e.Port) == port && e.PID > 0 {
			matches = append(matches, e)
		}
	}

	if len(matches) == 0 {
		return fmt.Errorf("no process found on port %d", port)
	}

	// Deduplicate by PID
	seen := make(map[int]bool)
	var unique []ports.PortInfo
	for _, m := range matches {
		if !seen[m.PID] {
			seen[m.PID] = true
			unique = append(unique, m)
		}
	}

	for _, m := range unique {
		sig := "SIGTERM"
		if flagForce {
			sig = "SIGKILL"
		}

		if !flagYes {
			fmt.Printf("Kill process %q (PID %d) on port %d with %s? [y/N] ", m.Process, m.PID, m.Port, sig)
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				fmt.Println("Skipped.")
				continue
			}
		}

		signal := syscall.SIGTERM
		if flagForce {
			signal = syscall.SIGKILL
		}

		if err := syscall.Kill(m.PID, signal); err != nil {
			return fmt.Errorf("killing PID %d: %w", m.PID, err)
		}
		fmt.Printf("Sent %s to %q (PID %d)\n", sig, m.Process, m.PID)
	}

	return nil
}

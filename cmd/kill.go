package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Viljoen13/port-explorer/internal/ports"
	"github.com/Viljoen13/port-explorer/internal/theme"
	"github.com/spf13/cobra"
)

var (
	flagForce bool
	flagYes   bool
	flagByPID bool
)

var killCmd = &cobra.Command{
	Use:   "kill <port|pid>...",
	Short: "Stop the process holding a port",
	Long: `Stop the process that owns a port. By default the process receives a polite
termination request; use --force to kill it outright.

Targets are ports. Pass --pid to treat them as process IDs instead.`,
	Example: `  port-explorer kill 3000
  port-explorer kill 3000 3001 --yes
  port-explorer kill 8080 --force
  port-explorer kill --pid 41234`,
	Args: cobra.MinimumNArgs(1),
	RunE: runKill,
}

func init() {
	killCmd.Flags().BoolVarP(&flagForce, "force", "f", false, "kill immediately (SIGKILL) instead of asking nicely (SIGTERM)")
	killCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "do not ask for confirmation")
	killCmd.Flags().BoolVar(&flagByPID, "pid", false, "treat arguments as process IDs rather than ports")
	rootCmd.AddCommand(killCmd)
}

func runKill(cmd *cobra.Command, args []string) error {
	entries, err := ports.List()
	if err != nil {
		return fmt.Errorf("listing ports: %w", err)
	}

	type target struct {
		pid     int
		process string
		port    uint16
	}
	var targets []target
	seen := map[int]bool{}
	self := os.Getpid()

	for _, arg := range args {
		n, err := strconv.Atoi(strings.TrimPrefix(arg, ":"))
		if err != nil || n <= 0 {
			return fmt.Errorf("invalid target %q: want a port number or PID", arg)
		}
		if flagByPID {
			name := ""
			for _, e := range entries {
				if e.PID == n {
					name = e.Process
					break
				}
			}
			if !seen[n] {
				seen[n] = true
				targets = append(targets, target{pid: n, process: name})
			}
			continue
		}
		found := false
		hidden := 0
		for _, e := range entries {
			if int(e.Port) != n {
				continue
			}
			found = true
			if e.PID == 0 {
				hidden++
				continue
			}
			if !seen[e.PID] {
				seen[e.PID] = true
				targets = append(targets, target{pid: e.PID, process: e.Process, port: e.Port})
			}
		}
		switch {
		case !found:
			return fmt.Errorf("nothing is using port %d", n)
		case len(targets) == 0 && hidden > 0:
			return fmt.Errorf("port %d is in use but its owner is hidden — rerun with sudo", n)
		}
	}

	for _, t := range targets {
		if t.pid == self {
			fmt.Println(theme.Warning.Render("Refusing to kill port-explorer itself."))
			continue
		}
		label := describe(t.process, t.pid, t.port)
		if !flagYes && !confirmPrompt(fmt.Sprintf("Send %s to %s?", ports.SignalName(flagForce), label)) {
			fmt.Println(theme.Muted.Render("Skipped."))
			continue
		}
		if err := ports.Kill(t.pid, flagForce); err != nil {
			return err
		}
		fmt.Printf("%s Sent %s to %s\n", theme.Success.Render("✓"), ports.SignalName(flagForce), label)
		if !flagForce {
			if stillAlive(t.pid, 1500*time.Millisecond) {
				fmt.Println(theme.Warning.Render("  still running after 1.5s — use --force if it refuses to exit"))
			}
		}
	}
	return nil
}

func describe(process string, pid int, port uint16) string {
	name := process
	if name == "" {
		name = "process"
	}
	s := fmt.Sprintf("%s (PID %d)", theme.Bold.Render(name), pid)
	if port > 0 {
		s += fmt.Sprintf(" on port %d", port)
	}
	return s
}

func confirmPrompt(question string) bool {
	fmt.Printf("%s [y/N] ", question)
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

// stillAlive polls the port table until the PID disappears or the deadline passes.
func stillAlive(pid int, wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
		entries, err := ports.List()
		if err != nil {
			return false
		}
		alive := false
		for _, e := range entries {
			if e.PID == pid {
				alive = true
				break
			}
		}
		if !alive {
			return false
		}
	}
	return true
}

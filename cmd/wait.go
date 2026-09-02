package cmd

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Viljoen13/port-explorer/internal/ports"
	"github.com/Viljoen13/port-explorer/internal/theme"
	"github.com/spf13/cobra"
)

var (
	flagWaitTimeout  time.Duration
	flagWaitInterval time.Duration
	flagWaitClosed   bool
	flagWaitQuiet    bool
)

var waitCmd = &cobra.Command{
	Use:   "wait <port>",
	Short: "Block until something starts (or stops) listening on a port",
	Long: `Poll until a process is listening on PORT, then exit 0. With --closed, wait
for the port to become free instead. Exit 1 on timeout.

Handy in scripts and CI:

  npm start &
  port-explorer wait 3000 --timeout 30s && npx cypress run`,
	Example: `  port-explorer wait 5432
  port-explorer wait 8080 --timeout 1m
  port-explorer wait 3000 --closed`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		n, err := strconv.ParseUint(args[0], 10, 16)
		if err != nil {
			return fmt.Errorf("invalid port %q", args[0])
		}
		port := uint16(n)
		start := time.Now()
		var deadline time.Time
		if flagWaitTimeout > 0 {
			deadline = start.Add(flagWaitTimeout)
		}
		if flagWaitInterval < 50*time.Millisecond {
			flagWaitInterval = 50 * time.Millisecond
		}

		for {
			entries, err := ports.List()
			if err != nil {
				return fmt.Errorf("listing ports: %w", err)
			}
			var owner *ports.PortInfo
			for i := range entries {
				if entries[i].Port == port && entries[i].IsListening() {
					owner = &entries[i]
					break
				}
			}
			if (owner != nil) != flagWaitClosed {
				if !flagWaitQuiet {
					elapsed := time.Since(start).Truncate(100 * time.Millisecond)
					if flagWaitClosed {
						fmt.Printf("%s port %d is free after %s\n", theme.Success.Render("✓"), port, elapsed)
					} else {
						who := "an unknown process"
						if owner.Process != "" {
							who = fmt.Sprintf("%s (PID %d)", owner.Process, owner.PID)
						}
						fmt.Printf("%s port %d is up: %s after %s\n", theme.Success.Render("✓"), port, who, elapsed)
					}
				}
				return nil
			}
			if !deadline.IsZero() && time.Now().After(deadline) {
				if !flagWaitQuiet {
					state := "open"
					if flagWaitClosed {
						state = "close"
					}
					fmt.Fprintf(os.Stderr, "%s timed out after %s waiting for port %d to %s\n", theme.Error.Render("✗"), flagWaitTimeout, port, state)
				}
				return errNoMatch
			}
			time.Sleep(flagWaitInterval)
		}
	},
}

func init() {
	waitCmd.Flags().DurationVarP(&flagWaitTimeout, "timeout", "t", 0, "give up after this long (0 = wait forever)")
	waitCmd.Flags().DurationVar(&flagWaitInterval, "interval", 250*time.Millisecond, "how often to poll")
	waitCmd.Flags().BoolVar(&flagWaitClosed, "closed", false, "wait for the port to be released instead")
	waitCmd.Flags().BoolVarP(&flagWaitQuiet, "quiet", "q", false, "print nothing; only set the exit status")
	rootCmd.AddCommand(waitCmd)
}

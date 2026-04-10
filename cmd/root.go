package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Viljoen13/port-explorer/internal/display"
	"github.com/Viljoen13/port-explorer/internal/ports"
	"github.com/Viljoen13/port-explorer/internal/tui"
	"github.com/spf13/cobra"
)

var (
	flagProcess string
	flagJSON    bool
	flagAll     bool
	flagList    bool
)

var rootCmd = &cobra.Command{
	Use:   "port-explorer [port or range]",
	Short: "See what's running on your ports",
	Long:  "A friendly, cross-platform tool to inspect network ports, processes, and connections.\n\nRun without flags for an interactive TUI. Use --list or --json for non-interactive output.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runRoot,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVarP(&flagProcess, "process", "p", "", "filter by process name (substring match)")
	rootCmd.Flags().BoolVarP(&flagJSON, "json", "j", false, "output as JSON (non-interactive)")
	rootCmd.Flags().BoolVarP(&flagAll, "all", "a", false, "show all connections, not just listening")
	rootCmd.Flags().BoolVarP(&flagList, "list", "l", false, "non-interactive table output")
}

func runRoot(cmd *cobra.Command, args []string) error {
	// Launch interactive TUI if no non-interactive flags are set
	if !flagJSON && !flagList && len(args) == 0 && flagProcess == "" {
		return tui.Run()
	}

	return runList(args)
}

func runList(args []string) error {
	entries, err := ports.List()
	if err != nil {
		return fmt.Errorf("listing ports: %w", err)
	}

	// Filter by listening only (default unless --all)
	if !flagAll {
		entries = filterState(entries, "LISTEN")
	}

	// Filter by port or port range
	if len(args) == 1 {
		entries, err = filterByPort(entries, args[0])
		if err != nil {
			return err
		}
	}

	// Filter by process name
	if flagProcess != "" {
		entries = filterByProcess(entries, flagProcess)
	}

	// Deduplicate: same port+proto+pid shown once (IPv4 and IPv6 both show up)
	entries = dedup(entries)

	if flagJSON {
		return display.PrintJSON(os.Stdout, entries)
	}

	display.PrintTable(os.Stdout, entries)
	fmt.Println(display.FormatSummary(entries))
	return nil
}

func filterState(entries []ports.PortInfo, state string) []ports.PortInfo {
	var out []ports.PortInfo
	for _, e := range entries {
		if e.State == state {
			out = append(out, e)
		}
	}
	return out
}

func filterByPort(entries []ports.PortInfo, spec string) ([]ports.PortInfo, error) {
	if strings.Contains(spec, "-") {
		parts := strings.SplitN(spec, "-", 2)
		low, err := strconv.ParseUint(parts[0], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid port range start: %s", parts[0])
		}
		high, err := strconv.ParseUint(parts[1], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid port range end: %s", parts[1])
		}
		var out []ports.PortInfo
		for _, e := range entries {
			if uint64(e.Port) >= low && uint64(e.Port) <= high {
				out = append(out, e)
			}
		}
		return out, nil
	}

	port, err := strconv.ParseUint(spec, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %s", spec)
	}
	var out []ports.PortInfo
	for _, e := range entries {
		if uint64(e.Port) == port {
			out = append(out, e)
		}
	}
	return out, nil
}

func filterByProcess(entries []ports.PortInfo, name string) []ports.PortInfo {
	name = strings.ToLower(name)
	var out []ports.PortInfo
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Process), name) {
			out = append(out, e)
		}
	}
	return out
}

func dedup(entries []ports.PortInfo) []ports.PortInfo {
	type key struct {
		port  uint16
		proto string
		pid   int
	}
	seen := make(map[key]bool)
	var out []ports.PortInfo
	for _, e := range entries {
		k := key{e.Port, e.Protocol, e.PID}
		if !seen[k] {
			seen[k] = true
			out = append(out, e)
		}
	}
	return out
}

// Package cmd wires up the command-line interface.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Viljoen13/port-explorer/internal/display"
	"github.com/Viljoen13/port-explorer/internal/ports"
	"github.com/Viljoen13/port-explorer/internal/theme"
	"github.com/Viljoen13/port-explorer/internal/tui"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
)

// version is overridden at build time via -ldflags "-X .../cmd.version=v1.2.3".
var version = "dev"

var (
	flagProcess     string
	flagJSON        bool
	flagAll         bool
	flagList        bool
	flagInteractive bool
	flagOutput      string
	flagSort        string
	flagDesc        bool
	flagWide        bool
	flagNoColor     bool
	flagQuiet       bool
	flagRefresh     time.Duration
)

// errNoMatch signals "ran fine, found nothing" so scripts get exit code 1
// without an error banner.
var errNoMatch = errors.New("no matching ports")

var rootCmd = &cobra.Command{
	Use:   "port-explorer [filter...]",
	Short: "See what's running on your ports",
	Long: `port-explorer shows which processes hold which ports, on Linux, macOS and Windows.

Run it with no arguments for an interactive dashboard. Give it a filter for a
quick answer in the terminal:

  port-explorer 8080            what's on port 8080?
  port-explorer 3000-4000       anything in this range?
  port-explorer nginx           ports held by nginx
  port-explorer exposed         listening on a public interface
  port-explorer docker          ports belonging to containers
  port-explorer --all estab     every established connection

Filter language (terms are ANDed, prefix ! to negate):
  8080  :8080  3000-4000  >1024  <=443  tcp  udp  listen  estab
  exposed  local  docker  unknown  pid:123  user:root  proc:node
  svc:http  addr:127.0.0.1  remote:443  any-free-text

Exit status is 1 when a filter matches nothing, so it works in scripts:
  port-explorer -q 8080 && echo "port 8080 is busy"`,
	Example: `  port-explorer
  port-explorer 5432
  port-explorer -o json listen
  port-explorer -i exposed
  port-explorer kill 3000
  port-explorer free 3000
  port-explorer wait 8080 --timeout 30s`,
	Args:          cobra.ArbitraryArgs,
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runRoot,
}

// Execute runs the CLI and exits with an appropriate status.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if errors.Is(err, errNoMatch) {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, theme.Error.Render("error:"), err)
		os.Exit(2)
	}
}

func init() {
	f := rootCmd.Flags()
	f.StringVarP(&flagProcess, "process", "p", "", "filter by process name (shorthand for proc:NAME)")
	f.BoolVarP(&flagAll, "all", "a", false, "include established and other non-listening sockets")
	f.BoolVarP(&flagList, "list", "l", false, "print a table instead of opening the dashboard")
	f.BoolVarP(&flagInteractive, "interactive", "i", false, "open the dashboard even when a filter is given")
	f.BoolVarP(&flagJSON, "json", "j", false, "shorthand for --output json")
	f.StringVarP(&flagOutput, "output", "o", "table", "output format: table, json, csv, plain")
	f.StringVarP(&flagSort, "sort", "s", "port", "sort by: port, process, pid, state, user")
	f.BoolVar(&flagDesc, "desc", false, "sort descending")
	f.BoolVarP(&flagWide, "wide", "w", false, "show user, uptime and full command line")
	f.BoolVar(&flagNoColor, "no-color", false, "disable colours (also honours NO_COLOR)")
	f.BoolVarP(&flagQuiet, "quiet", "q", false, "print nothing; only set the exit status")
	f.DurationVar(&flagRefresh, "refresh", 2*time.Second, "auto-refresh interval for live mode in the dashboard")

	rootCmd.PersistentPreRun = func(*cobra.Command, []string) {
		if flagNoColor || os.Getenv("NO_COLOR") != "" {
			lipgloss.SetColorProfile(termenv.Ascii)
		}
	}
	rootCmd.SetVersionTemplate("port-explorer {{.Version}}\n")
}

func buildQuery(args []string) string {
	terms := append([]string{}, args...)
	if flagProcess != "" {
		terms = append(terms, "proc:"+flagProcess)
	}
	return strings.Join(terms, " ")
}

func runRoot(cmd *cobra.Command, args []string) error {
	query := buildQuery(args)
	nonInteractive := flagList || flagJSON || flagQuiet || cmd.Flags().Changed("output") || query != ""
	if flagInteractive || !nonInteractive {
		return tui.Run(tui.Options{Query: query, ShowAll: flagAll, Refresh: flagRefresh})
	}
	return runList(query)
}

func runList(query string) error {
	if flagJSON {
		flagOutput = "json"
	}
	sortField, err := parseSortField(flagSort)
	if err != nil {
		return err
	}

	entries, err := ports.List()
	if err != nil {
		return fmt.Errorf("listing ports: %w", err)
	}
	entries = ports.Merge(entries)
	if !flagAll {
		entries = ports.Listening(entries)
	}
	q := ports.ParseQuery(query)
	entries = q.Filter(entries)
	ports.Sort(entries, sortField, !flagDesc)

	if flagQuiet {
		if len(entries) == 0 {
			return errNoMatch
		}
		return nil
	}

	switch flagOutput {
	case "json":
		if err := display.PrintJSON(os.Stdout, entries); err != nil {
			return err
		}
	case "csv":
		if err := display.PrintCSV(os.Stdout, entries); err != nil {
			return err
		}
	case "plain":
		display.PrintPlain(os.Stdout, entries)
	case "table":
		if len(entries) == 0 {
			printNothingFound(q)
		} else {
			display.PrintTable(os.Stdout, entries, display.Options{ShowRemote: flagAll, Wide: flagWide})
			fmt.Println()
			fmt.Println(display.Summary(entries, ports.IsPrivileged()))
		}
	default:
		return fmt.Errorf("unknown output format %q (want table, json, csv or plain)", flagOutput)
	}

	if len(entries) == 0 && !q.Empty() {
		return errNoMatch
	}
	return nil
}

func printNothingFound(q ports.Query) {
	if port, ok := q.ExactPort(); ok {
		fmt.Printf("%s Nothing is listening on port %d — it's free.\n", theme.Success.Render("✓"), port)
		return
	}
	if q.Empty() {
		fmt.Println(theme.Muted.Render("No sockets found."))
		return
	}
	fmt.Printf("%s\n", theme.Muted.Render(fmt.Sprintf("No ports match %q.", q.String())))
	if !flagAll {
		fmt.Println(theme.Muted.Render("  Only listening sockets are shown; add --all to include established connections."))
	}
}

func parseSortField(s string) (ports.SortField, error) {
	for _, f := range ports.SortFields {
		if string(f) == strings.ToLower(s) {
			return f, nil
		}
	}
	return "", fmt.Errorf("unknown sort field %q (want port, process, pid, state or user)", s)
}

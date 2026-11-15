// Command ine browses and bulk-downloads the tables of the INE, the Spanish
// statistics office, through its Tempus3 JSON API.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
)

const usage = `ine: browse and bulk-download INE tables (Tempus3 API).

Usage:
  ine ops    [--json] [filter]             list statistical operations
  ine tables [--json] <operation> [filter] list the tables of an operation
  ine get    [--json] [period flags] <table>   show the series of a table
  ine series [--json] [period flags] <code>    show one series
  ine fetch  [flags] [table...]            download many tables to disk
  ine tui    [-o dir]                      browse, pick tables and download them
  ine version                              print the version

Period flags:
  --last N            only the last N periods
  --from YYYY-MM-DD   first period
  --to   YYYY-MM-DD   last period

Every command takes --lang ES|EN. Run "ine <command> -h" for its flags.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		if !errors.Is(err, errUsage) {
			fmt.Fprintln(os.Stderr, "ine:", err)
		}
		os.Exit(1)
	}
}

var errUsage = errors.New("usage")

// version is set by the release build (-ldflags "-X main.version=...").
var version = ""

// buildVersion falls back to the module version that go install records.
func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "dev"
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(usage)
		if len(args) == 0 {
			return errUsage
		}
		return nil
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "ops":
		return cmdOps(ctx, rest)
	case "tables":
		return cmdTables(ctx, rest)
	case "get":
		return cmdGet(ctx, rest)
	case "series":
		return cmdSeries(ctx, rest)
	case "fetch":
		return cmdFetch(ctx, rest)
	case "tui":
		return cmdTUI(ctx, rest)
	case "version", "--version", "-v":
		fmt.Println("ine", buildVersion())
		return nil
	default:
		fmt.Fprintf(os.Stderr, "ine: unknown command %q\n\n%s", cmd, usage)
		return errUsage
	}
}

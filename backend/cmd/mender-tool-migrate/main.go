// mender-tool-migrate assesses or stages an offline, forward-only Tool
// ownership cut. It NEVER swaps, upgrades, restarts or writes the source DB.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	migration "github.com/orz-i/Mender/backend/internal/migration/toolhardcut"
)

func main() { os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr)) }

func run(ctx context.Context, args []string, out, diagnostics io.Writer) int {
	if len(args) == 0 {
		printUsage(diagnostics)
		return 2
	}
	switch args[0] {
	case "assess":
		set := flag.NewFlagSet("assess", flag.ContinueOnError)
		set.SetOutput(diagnostics)
		db := set.String("db", "", "existing SQLite database (read-only)")
		format := set.String("format", "json", "report format (json only)")
		if err := set.Parse(args[1:]); err != nil {
			return 2
		}
		if set.NArg() != 0 || strings.TrimSpace(*db) == "" || *format != "json" {
			_, _ = fmt.Fprintln(diagnostics, "assess requires --db and --format=json, with no positional arguments")
			return 2
		}
		report, err := migration.AssessPath(ctx, *db)
		if err != nil {
			_, _ = fmt.Fprintln(diagnostics, "assessment failed:", err)
			return 1
		}
		if err := encode(out, report); err != nil {
			_, _ = fmt.Fprintln(diagnostics, "report output failed:", err)
			return 1
		}
		// Reports with blockers or pending operator review must be a nonzero
		// CI/automation gate, despite supplying their full structured evidence.
		if report.Status != "ready" {
			return 3
		}
		return 0
	case "stage-copy":
		set := flag.NewFlagSet("stage-copy", flag.ContinueOnError)
		set.SetOutput(diagnostics)
		db := set.String("db", "", "existing SQLite database (read-only)")
		dest := set.String("destination", "", "new candidate SQLite database path")
		manifest := set.String("manifest", "", "exact reviewed SHA-256 manifest digest")
		if err := set.Parse(args[1:]); err != nil {
			return 2
		}
		if set.NArg() != 0 || strings.TrimSpace(*db) == "" || strings.TrimSpace(*dest) == "" ||
			!strings.HasPrefix(*manifest, "sha256:") {
			_, _ = fmt.Fprintln(diagnostics, "stage-copy requires --db, --destination, and --manifest=sha256:<reviewed-digest>")
			return 2
		}
		artifact, err := migration.StageCopy(ctx, *db, *dest, *manifest)
		if err != nil {
			_, _ = fmt.Fprintln(diagnostics, "staging refused:", err)
			return 3
		}
		if err := encode(out, artifact); err != nil {
			_, _ = fmt.Fprintln(diagnostics, "staging report failed:", err)
			return 1
		}
		return 0
	case "-h", "--help", "help":
		printUsage(out)
		return 0
	default:
		_, _ = fmt.Fprintln(diagnostics, "unknown command:", args[0])
		printUsage(diagnostics)
		return 2
	}
}

func encode(out io.Writer, value any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(true)
	return enc.Encode(value)
}
func printUsage(to io.Writer) {
	_, _ = fmt.Fprintln(to, `Usage: mender-tool-migrate assess --db PATH [--format json]
       mender-tool-migrate stage-copy --db PATH --destination NEW_PATH --manifest sha256:DIGEST

assess reads a consistent snapshot only; status blocked/requires_review exits 3.
stage-copy creates an independent backup and candidate; it NEVER upgrades the
source, changes running Mender, grants Tools or swaps production databases.
Unreviewed Profile/MCP changes and unverified Agent/SSH Jobs block staging.`)
}

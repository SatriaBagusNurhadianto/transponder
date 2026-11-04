// Command transponder is an LLM gateway with fallback, per-user cost tracking and budgets.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("transponder", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		_, _ = fmt.Fprintf(stdout, "transponder %s\n", version)
		return 0
	}
	_, _ = fmt.Fprintln(stderr, "transponder is in development; the gateway is not implemented yet")
	return 1
}

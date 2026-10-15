// Command squad is the single Squad binary: the local server (`squad serve`)
// and the CLI that agents use from inside Squad terminals (`squad ask …`).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/nathanribeiroo/squad/internal/version"
)

type command struct {
	name    string
	summary string
	run     func(args []string, stdout, stderr io.Writer) int
}

func commands() []command {
	return []command{
		{"serve", "start the local server and open the canvas", runServe},
		{"list", "list nodes connected to this terminal", runClient},
		{"ask", "send a message to a connected agent and wait for the reply", runClient},
		{"check", "read a connected terminal's current screen", runClient},
		{"reply", "answer a pending request by id", runClient},
		{"note", "read or write connected notes", runClient},
		{"hook", "receive an agent hook (used by agent configs)", runHook},
		{"version", "print the version", runVersion},
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage(stdout)
		return 0
	}
	for _, c := range commands() {
		if c.name == args[0] {
			return c.run(args, stdout, stderr)
		}
	}
	fmt.Fprintf(stderr, "squad: unknown command %q\n\n", args[0])
	usage(stderr)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: squad <command> [arguments]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range commands() {
		fmt.Fprintf(w, "  %-8s %s\n", c.name, c.summary)
	}
}

func runVersion(_ []string, stdout, _ io.Writer) int {
	v := version.Version
	if version.Commit != "" {
		v += " (" + version.Commit + ")"
	}
	fmt.Fprintln(stdout, "squad", v)
	return 0
}

func runClient(args []string, _, stderr io.Writer) int {
	fmt.Fprintf(stderr, "squad %s: not implemented yet\n", args[0])
	return 1
}

func runHook(_ []string, _, stderr io.Writer) int {
	fmt.Fprintln(stderr, "squad hook: not implemented yet")
	return 1
}

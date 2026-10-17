// Package cli is the torre command: the local server (`torre serve`) and
// the CLI that agents use from inside Torre terminals (`torre ask …`).
// cmd/torre builds it; cmd/squad builds the same command under the name it
// had before Squad became Torre.
package cli

import (
	"fmt"
	"io"

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
		{"wait", "wait for a connected agent's turn to end and get its final answer", runClient},
		{"reply", "answer a pending request by id", runClient},
		{"note", "read or write connected notes", runClient},
		{"portal", "use a connected portal (browser): snapshot, click, fill…", runClient},
		{"roles", "the roles agents can have", runClient},
		{"new", "managers: create a terminal next to you, connected and started", runClient},
		{"connect", "managers: connect two nodes of the canvas", runClient},
		{"close", "managers: remove a terminal you created", runClient},
		{"floor", "floors (git worktrees with their own canvas): list, create, land", runClient},
		{"mcp", "serve the commands above as MCP tools over stdio (agents start it)", runMCP},
		{"hook", "receive an agent hook (used by agent configs)", runHook},
		{"install-skill", "install the Torre skill for Claude Code and Devin (and Devin's MCP server)", runInstallSkill},
		{"version", "print the version", runVersion},
	}
}

// Main runs the torre command with args (without the program name).
func Main(args []string, stdout, stderr io.Writer) int {
	return run(args, stdout, stderr)
}

// Legacy runs it for the squad command, the name from before Squad became
// Torre: the same commands, with a word about the new name for whoever
// types it (not for agent hooks and the MCP server, which agents start).
func Legacy(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "hook" && args[0] != "mcp" {
		fmt.Fprintln(stderr, "squad is now torre: use the torre command (squad keeps working for now).")
	}
	return run(args, stdout, stderr)
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
	fmt.Fprintf(stderr, "torre: unknown command %q\n\n", args[0])
	usage(stderr)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: torre <command> [arguments]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range commands() {
		fmt.Fprintf(w, "  %-14s %s\n", c.name, c.summary)
	}
}

func runVersion(_ []string, stdout, _ io.Writer) int {
	fmt.Fprintln(stdout, "torre", versionString())
	return 0
}

// versionString is the version, with the commit when the build knows it.
func versionString() string {
	v := version.Version
	if version.Commit != "" {
		v += " (" + version.Commit + ")"
	}
	return v
}

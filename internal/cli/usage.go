package cli

import (
	"fmt"
	"io"
	"runtime"

	agentusage "github.com/nathanribeiroo/squad/internal/usage"
)

// runUsageReport is `torre usage-report`: what Torre reads about the
// agents' usage on this computer, numbers only, for the user to send when
// the Consumo box doesn't add up.
func runUsageReport(args []string, stdout, stderr io.Writer) int {
	if len(args) > 1 {
		if args[1] == "-h" || args[1] == "--help" {
			fmt.Fprintln(stdout, "Usage: torre usage-report")
			fmt.Fprintln(stdout)
			fmt.Fprintln(stdout, "Prints what Torre can read about Devin's and Claude Code's usage on this computer:")
			fmt.Fprintln(stdout, "Devin's sessions.db and user_status.bin, Claude Code's conversations and provider.")
			fmt.Fprintln(stdout, "Numbers only: no text from the files is printed (no e-mail, team or conversation).")
			return 0
		}
		fmt.Fprintln(stderr, "torre usage-report takes no arguments")
		return 2
	}
	fmt.Fprintf(stdout, "torre %s · %s/%s\n", versionString(), runtime.GOOS, runtime.GOARCH)
	agentusage.Report(stdout, agentusage.DefaultReportOptions())
	return 0
}

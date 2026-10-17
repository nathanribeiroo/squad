// Command squad is torre under the name it had before Squad became Torre,
// so scripts, agent configs and conversations from before keep working.
package main

import (
	"os"

	"github.com/nathanribeiroo/squad/internal/cli"
)

func main() {
	os.Exit(cli.Legacy(os.Args[1:], os.Stdout, os.Stderr))
}

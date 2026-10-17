// Command torre is the single Torre binary: the local server (`torre serve`)
// and the CLI that agents use from inside Torre terminals (`torre ask …`).
package main

import (
	"os"

	"github.com/nathanribeiroo/squad/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}

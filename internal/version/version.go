// Package version holds build metadata, set at link time with
// -ldflags "-X github.com/nathanribeiroo/squad/internal/version.Version=v0.1.0".
package version

// Version is the release version; "dev" for local builds.
var Version = "dev"

// Commit is the git commit the binary was built from.
var Commit = ""

// Package version holds build metadata, set at link time with
// -ldflags "-X github.com/nathanribeiroo/squad/internal/version.Version=v0.1.0".
// Without ldflags (e.g. `go install …@v0.1.0`) it falls back to the module
// version and VCS revision the Go toolchain records in the binary.
package version

import "runtime/debug"

// Version is the release version; "dev" for local builds.
var Version = "dev"

// Commit is the git commit the binary was built from.
var Commit = ""

func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if Version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = info.Main.Version
	}
	if Commit == "" {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				Commit = s.Value[:7]
			}
		}
	}
}

// Package version holds build information, injected at build time with -ldflags.
package version

var (
	// Version is the semantic version of this build, for example "v0.1.0" or "v0.1.0-rc.1".
	Version = "v0.0.0-dev"
	// Commit is the git commit this build was made from.
	Commit = "unknown"
	// BuildDate is the UTC build time in RFC 3339 format.
	BuildDate = ""
)

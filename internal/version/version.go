// Package version provides build-time version information injected via ldflags.
//
// Defaults apply when running with go run or building without ldflags:
//
//	go build -ldflags "\
//	  -X crobot/internal/version.Version=$(git describe --tags --always --dirty) \
//	  -X crobot/internal/version.Commit=$(git rev-parse --short HEAD) \
//	  -X crobot/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
//	  -o build/agent ./cmd/agent
package version

var (
	// Version is the semantic version or git describe output (e.g. "v0.1.0" or "abc1234").
	Version = "dev"
	// Commit is the short Git commit hash.
	Commit = "unknown"
	// BuildDate is the UTC build timestamp in ISO 8601 format.
	BuildDate = "unknown"
)

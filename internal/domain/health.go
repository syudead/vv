package domain

import "time"

// Override DefaultVersion at build time with -ldflags "-X main.version=…".
const DefaultVersion = "dev"

type Status string

const (
	StatusOK       Status = "ok"
	StatusDegraded Status = "degraded"
)

func (s Status) Valid() bool {
	switch s {
	case StatusOK, StatusDegraded:
		return true
	default:
		return false
	}
}

// Commit and BuiltAt are empty when unavailable.
type BuildInfo struct {
	Version string
	Commit  string
	BuiltAt time.Time
}

type Health struct {
	Status  Status
	Version string
	Commit  string
	BuiltAt time.Time
}

// NewHealth evaluates storage reachability per request without retaining state.
func NewHealth(build BuildInfo, storeReachable bool) Health {
	status := StatusDegraded
	if storeReachable {
		status = StatusOK
	}

	version := build.Version
	if version == "" {
		version = DefaultVersion
	}

	return Health{
		Status:  status,
		Version: version,
		Commit:  build.Commit,
		BuiltAt: build.BuiltAt,
	}
}

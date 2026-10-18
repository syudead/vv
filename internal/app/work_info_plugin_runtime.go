package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// WorkInfoPluginRuntime inspects and executes WebAssembly plugins; implemented by wasmplugin.Runtime.
type WorkInfoPluginRuntime interface {
	// Inspect never executes the module and returns a manifest or unusability issue, preserving a readable manifest alongside later validation issues. Oversized modules return ErrWorkInfoPluginTooLarge.
	Inspect(ctx context.Context, module []byte) (domain.WorkInfoPluginManifest, domain.WorkInfoPluginProblem, error)
	// Run executes the digest in a fresh instance with the selected network and returns WorkInfoRunError on failure. Return stderr logs on success or failure and invoke optional record before releasing the execution slot when the module finishes.
	Run(
		ctx context.Context, digest string, module []byte, req domain.WorkInfoRunRequest, network domain.WorkInfoRunNetwork,
		record func(domain.WorkInfoRunLog) error,
	) (domain.WorkInfoRunResult, domain.WorkInfoRunLog, error)
}

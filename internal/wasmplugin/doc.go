// Package wasmplugin inspects and runs isolated WASI modules with bounded time, memory, output, and
// concurrency, exposing only standard streams, clocks, randomness, and controlled HTTP; see
// docs/design-docs/work-info-plugins.md. Only domain is an internal dependency; see
// ARCHITECTURE.md.
package wasmplugin

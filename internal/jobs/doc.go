// Package jobs runs the background queue workers without external queue middleware: one Loop per worker, and the
// scheduler that grants ingest jobs from shared capacity on top of it; only domain is an internal dependency. See
// ARCHITECTURE.md.
package jobs

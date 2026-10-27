// Package jobs runs the background queue workers without external queue middleware: one Loop per worker, and the
// ingest stage workers on top of it; only domain is an internal dependency. See ARCHITECTURE.md.
package jobs

// Package watcher reports changes in the media folders as changed directories.
//
// It wraps fsnotify with one watch per directory. Arming reads directory entries only and never opens a file.
// Each change is reported as a directory and a recursive flag; lost
// events, watch limits, permission errors and unreachable media folders are
// reported as problems and never repaired by a scan.
//
// The package depends on no other internal package. internal/app declares the
// port it uses and cmd/mdm wires this adapter to it.
package watcher

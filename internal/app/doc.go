// Package app owns use cases and declares interfaces implemented by adapters wired in cmd/mdm;
// depguard forbids HTTP, SQL, process, adapter, and eventbus imports. Dependencies flow cmd ->
// app/adapters -> domain; see ARCHITECTURE.md.
package app

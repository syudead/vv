// Package eventbus dispatches domain events through publisher-owned interfaces; cmd/mdm wires the
// bus and subscribers so publishers need not change when subscribers are added. Only domain is an
// internal dependency; see ARCHITECTURE.md.
package eventbus

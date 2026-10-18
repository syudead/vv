// Package mediafs centralizes regular-file access within registered roots, including resolved
// symlinks; consumers use their own interfaces and domain owns pure containment rules. Only domain
// is an internal dependency; see ARCHITECTURE.md.
package mediafs

// FS is stateless and its zero value is usable.
type FS struct{}

func New() FS { return FS{} }

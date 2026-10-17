package plugintest

import (
	"bytes"
	_ "embed"
)

// These small WASI modules exercise the real stdin/stdout and host-function
// boundary without compiling a Go runtime for every test runtime. Sources and
// the regeneration script live beside the binaries; tests need no C compiler.

//go:embed testdata/probe.wasm
var probe []byte

//go:embed testdata/cli.wasm
var cli []byte

// Probe returns the fixture for runtime, HTTP, proxy and cover-image tests.
func Probe() []byte { return bytes.Clone(probe) }

// CLI returns the fixture for the plugin run command's request and output tests.
func CLI() []byte { return bytes.Clone(cli) }

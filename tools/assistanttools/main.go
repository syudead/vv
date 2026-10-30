// Command assistanttools writes the assistant's tool registry from api/openapi.yaml; task generate runs it.
// See docs/design-docs/assistant.md for what the registry holds and who reads it.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	spec := flag.String("spec", "api/openapi.yaml", "the screen API document to read")
	out := flag.String("out", "internal/httpapi/gen/assistant_tools.gen.json", "the registry file to write")
	flag.Parse()

	registry, err := Load(*spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoded, err := Encode(registry)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, encoded, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Encode writes one operation per line group, sorted by id, so a spec change shows as a small diff.
func Encode(registry Registry) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(registry); err != nil {
		return nil, fmt.Errorf("encode the assistant tool registry: %w", err)
	}
	return buf.Bytes(), nil
}

//go:build !(windows && desktop)

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

// The server entry point reads MDM_* settings; desktop_windows.go provides the desktop-tagged entry
// point.
func main() {
	if len(os.Args) > 1 {
		os.Exit(runCommand(context.Background(), os.Args[1:], osAccountEnv()))
	}
	cfg, err := LoadConfig(os.Getenv)
	if err == nil {
		err = run(runOptions{
			Config:     cfg,
			LogOutput:  os.Stdout,
			Listener:   newReopenableListener(),
			NotifyStop: notifySignals,
		})
	}
	if err != nil {
		// Startup can fail before logging is configured, so report errors to stderr.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func osAccountEnv() accountEnv {
	env := accountEnv{Getenv: os.Getenv, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Now: time.Now}
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		env.ReadHidden = func() ([]byte, error) { return term.ReadPassword(fd) }
	}
	return env
}

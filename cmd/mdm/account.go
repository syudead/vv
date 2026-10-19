package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/password"
	"github.com/syudead/vv/internal/store"
	"github.com/syudead/vv/internal/wasmplugin"
)

// Shared exit codes for account and plugin subcommands.
const (
	exitAccountOK    = 0
	exitAccountError = 1 // Database access failure.
	exitAccountUsage = 2 // Invalid input, unconfigured account, or unknown subcommand.
)

const accountUsage = `Usage:
  mdm                                 start the server
  mdm account set-username <NAME>     change the username
  mdm account set-password            reset the password (read from standard input)
  mdm plugin pack <module.wasm> <manifest.json>
                                      check a work-info plugin manifest and write it into the module
  mdm plugin run <module.wasm> <operation> <input> [--set KEY=VALUE]... [--proxy URL] [--source SOURCE]
                                      run a work-info or person plugin once as VVMDM would; <operation>
                                      is search (input: the words or the person's name) or fetch
                                      (input: the work's or the person's ID; a person plugin's fetch
                                      needs --source); after --, every argument is positional, so
                                      words may start with --

Account settings are read only from MDM_DATA_DIR; plugin commands do not read it. The password is never taken from arguments or environment variables.
If standard input is a terminal, the password is asked for twice; otherwise the first line is used as the password.`

// accountEnv supplies replaceable host dependencies.
type accountEnv struct {
	Getenv func(string) string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// ReadHidden reads a terminal line without echo; nil means non-terminal input.
	ReadHidden func() ([]byte, error)
	Now        func() time.Time
	// PluginRuntime defaults to production networking; tests can replace resolution and dialing.
	PluginRuntime wasmplugin.Options
}

type accountFailure struct {
	code      int
	message   string
	withUsage bool
}

func usageFailure(format string, args ...any) *accountFailure {
	return &accountFailure{code: exitAccountUsage, message: fmt.Sprintf(format, args...), withUsage: true}
}

func operationFailure(format string, args ...any) *accountFailure {
	return &accountFailure{code: exitAccountError, message: fmt.Sprintf(format, args...)}
}

// Commands cannot create an account.
func notConfiguredFailure() *accountFailure {
	return usageFailure("The account is not set up yet. Open VVMDM in a browser and complete the initial setup first.")
}

// runCommand handles os.Args[1:] for CLI subcommands; server startup bypasses it.
func runCommand(ctx context.Context, args []string, env accountEnv) int {
	var failure *accountFailure
	switch {
	case len(args) > 0 && args[0] == "plugin":
		failure = runPlugin(ctx, args[1:], env)
	case len(args) == 0 || args[0] != "account":
		failure = usageFailure("Unknown command: %s", strings.Join(args, " "))
	case len(args) == 1:
		failure = usageFailure("Specify an account subcommand.")
	default:
		failure = runAccount(ctx, args[1], args[2:], env)
	}

	if failure == nil {
		return exitAccountOK
	}
	_, _ = fmt.Fprintln(env.Stderr, failure.message)
	if failure.withUsage {
		_, _ = fmt.Fprintln(env.Stderr)
		_, _ = fmt.Fprintln(env.Stderr, accountUsage)
	}
	return failure.code
}

// runAccount reports successful changes to stderr without exposing passwords or hashes.
func runAccount(ctx context.Context, sub string, args []string, env accountEnv) *accountFailure {
	switch sub {
	case "set-username":
		if len(args) != 1 {
			return usageFailure("set-username takes exactly one argument: the new username.")
		}
		username := args[0]
		if err := domain.ValidateUsername(username); err != nil {
			return usageFailure("%v. Use 1 to %d characters, with no control characters and no leading or trailing spaces.",
				err, domain.MaxUsernameLength)
		}
		return withAuthStore(ctx, env, func(auth *store.AuthStore) *accountFailure {
			return changeCredentials(auth.ChangeUsername(ctx, username, env.Now()), "Changed the username. ", env)
		})

	case "set-password":
		if len(args) != 0 {
			return usageFailure("set-password takes no arguments. Pass the password on standard input.")
		}
		return withAuthStore(ctx, env, func(auth *store.AuthStore) *accountFailure {
			// Reject unconfigured accounts before prompting; recheck when writing.
			if _, err := auth.Account(ctx); errors.Is(err, domain.ErrAccountNotConfigured) {
				return notConfiguredFailure()
			} else if err != nil {
				return operationFailure("%v", err)
			}
			secret, failure := readNewPassword(env)
			if failure != nil {
				return failure
			}
			hash, err := password.Hash(secret)
			if err != nil {
				return operationFailure("%v", err)
			}
			return changeCredentials(auth.ChangePassword(ctx, hash, env.Now()), "Reset the password. ", env)
		})

	default:
		return usageFailure("Unknown account subcommand: %s", sub)
	}
}

func changeCredentials(err error, done string, env accountEnv) *accountFailure {
	if errors.Is(err, domain.ErrAccountNotConfigured) {
		return notConfiguredFailure()
	}
	if err != nil {
		return operationFailure("%v", err)
	}
	_, _ = fmt.Fprintln(env.Stderr, done+"All existing login sessions have been signed out and all API tokens have been revoked.")
	return nil
}

// withAuthStore opens an existing database and migrates it without checking media tools. Missing
// files fail as unconfigured, so a mistyped data path cannot create a database.
func withAuthStore(ctx context.Context, env accountEnv, use func(*store.AuthStore) *accountFailure) *accountFailure {
	dataDir := valueOr(env.Getenv(envDataDir), defaultDataDir)
	if !filepath.IsAbs(dataDir) {
		return usageFailure("%s=%q is not an absolute path.", envDataDir, dataDir)
	}
	if _, err := os.Stat(store.DatabasePath(dataDir)); errors.Is(err, os.ErrNotExist) {
		return notConfiguredFailure()
	} else if err != nil {
		return operationFailure("Cannot check the database: %v", err)
	}

	db, err := store.OpenContext(ctx, dataDir)
	if err != nil {
		return operationFailure("%v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := store.Migrate(ctx, db); err != nil {
		return operationFailure("%v", err)
	}
	return use(db.Auth())
}

// readNewPassword validates input, prompting twice without echo on terminals or reading one line
// otherwise.
func readNewPassword(env accountEnv) (string, *accountFailure) {
	var secret string
	if env.ReadHidden != nil {
		first, err := promptHidden(env, "New password: ")
		if err != nil {
			return "", operationFailure("Cannot read the password: %v", err)
		}
		second, err := promptHidden(env, "Enter it again: ")
		if err != nil {
			return "", operationFailure("Cannot read the password: %v", err)
		}
		if first != second {
			return "", usageFailure("The two entries do not match. Nothing was changed.")
		}
		secret = first
	} else {
		line, err := readFirstLine(env.Stdin)
		if err != nil {
			return "", operationFailure("Cannot read the password: %v", err)
		}
		secret = line
	}

	if err := domain.ValidatePassword(secret); err != nil {
		return "", usageFailure("%v. Use 1 to %d bytes. Nothing was changed.", err, domain.MaxPasswordBytes)
	}
	return secret, nil
}

func promptHidden(env accountEnv, prompt string) (string, error) {
	_, _ = fmt.Fprint(env.Stderr, prompt)
	secret, err := env.ReadHidden()
	// Echo is disabled, so print the missing newline.
	_, _ = fmt.Fprintln(env.Stderr)
	if err != nil {
		return "", err
	}
	return string(secret), nil
}

// readFirstLine strips LF or CRLF and stops after enough bytes to detect an oversized password.
func readFirstLine(r io.Reader) (string, error) {
	// Read the limit plus CRLF and one extra byte to detect overflow.
	limited := io.LimitReader(r, int64(domain.MaxPasswordBytes)+3)
	line, err := bufio.NewReader(limited).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	// Strip CR only as part of CRLF; a trailing CR without LF belongs to the password.
	if trimmed, ok := strings.CutSuffix(line, "\n"); ok {
		line = strings.TrimSuffix(trimmed, "\r")
	}
	return line, nil
}

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

	"golang.org/x/term"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/password"
	"github.com/syudead/vv/internal/store"
)

// account の下位コマンドの終了コード（specs/016-single-account-auth/contracts/account-cli.md §3）。
const (
	exitAccountOK    = 0
	exitAccountError = 1 // DB を開けない・書けない
	exitAccountUsage = 2 // 未設定・値の誤り・確認の不一致・未知の下位コマンド
)

// accountUsage は account の下位コマンドの使い方である。
const accountUsage = `Usage:
  mdm                                 start the server
  mdm account set-username <NAME>     change the username
  mdm account set-password            reset the password (read from standard input)

Settings are read only from MDM_DATA_DIR. The password is never taken from arguments or environment variables.
If standard input is a terminal, the password is asked for twice; otherwise the first line is used as the password.`

// accountEnv はホスト側のコマンドが外界から受け取るものである。テストが差し替える。
type accountEnv struct {
	Getenv func(string) string
	Stdin  io.Reader
	Stderr io.Writer
	// ReadHidden はエコーを止めて端末から1行読む。nil なら標準入力は端末でない。
	ReadHidden func() ([]byte, error)
	Now        func() time.Time
}

// osAccountEnv は実際の標準入出力と環境変数を使う accountEnv を返す。
func osAccountEnv() accountEnv {
	env := accountEnv{Getenv: os.Getenv, Stdin: os.Stdin, Stderr: os.Stderr, Now: time.Now}
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		env.ReadHidden = func() ([]byte, error) { return term.ReadPassword(fd) }
	}
	return env
}

// accountFailure は終了コードと、標準エラーへ出す理由の組である。
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

// notConfiguredFailure は未設定のときの失敗である。コマンドではアカウントを作らない。
func notConfiguredFailure() *accountFailure {
	return usageFailure("The account is not set up yet. Open VVMDM in a browser and complete the initial setup first.")
}

// runCommand は引数つきの mdm を実行し、終了コードを返す。args は os.Args[1:] である。
// 引数なしの起動（サーバー）はここを通らない。
func runCommand(ctx context.Context, args []string, env accountEnv) int {
	var failure *accountFailure
	switch {
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

// runAccount は account の下位コマンド sub を実行する。成功したら何を変えたかを
// 標準エラーへ出して nil を返す。パスワードとそのハッシュはどの結果でも出力しない。
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
			// 未設定なら尋ねる前に終える。書き換えでも未設定は確かめ直す。
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

// changeCredentials は書き換えの結果を終了の形にする。
func changeCredentials(err error, done string, env accountEnv) *accountFailure {
	if errors.Is(err, domain.ErrAccountNotConfigured) {
		return notConfiguredFailure()
	}
	if err != nil {
		return operationFailure("%v", err)
	}
	_, _ = fmt.Fprintln(env.Stderr, done+"All existing login sessions have been signed out.")
	return nil
}

// withAuthStore は MDM_DATA_DIR のデータベースを開き、マイグレーションを適用してから
// use に AuthStore を渡す。サーバーの起動前確認（ffmpeg・ffprobe）は行わない。
//
// データベースのファイルが無ければ未設定として終える。ファイルを作らないので、
// MDM_DATA_DIR の打ち間違いで空のデータベースができることもない。
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

// readNewPassword は新しいパスワードを受け取り、値の規則で確かめる
// （contracts/account-cli.md §2）。端末ならエコーを止めて2回尋ね、端末でなければ
// 最初の1行（末尾の改行を除く）を使う。
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

// promptHidden は標準エラーに prompt を出し、エコーを止めて1行読む。
func promptHidden(env accountEnv, prompt string) (string, error) {
	_, _ = fmt.Fprint(env.Stderr, prompt)
	secret, err := env.ReadHidden()
	// エコーを止めているので、入力の改行は画面に出ない。
	_, _ = fmt.Fprintln(env.Stderr)
	if err != nil {
		return "", err
	}
	return string(secret), nil
}

// readFirstLine は r の最初の1行を、末尾の改行（\n または \r\n）を除いて返す。
// 上限を超える長さは読み切らず、上限を超えた値として返す（規則の確認で外れる）。
func readFirstLine(r io.Reader) (string, error) {
	// 上限 + 改行2バイト + 1 まで読めば、上限を超えたかを判断できる。
	limited := io.LimitReader(r, int64(domain.MaxPasswordBytes)+3)
	line, err := bufio.NewReader(limited).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	// CR を除くのは CRLF の一部のときだけ。改行なしで終わる値の末尾の CR はパスワードの一部。
	if trimmed, ok := strings.CutSuffix(line, "\n"); ok {
		line = strings.TrimSuffix(trimmed, "\r")
	}
	return line, nil
}

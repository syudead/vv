package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/password"
	"github.com/syudead/vv/internal/store"
)

const (
	oldUsername = "alice"
	oldPassword = "old-secret-パスワード"
	newUsername = "bob"
	newPassword = "new-secret-パスワード"
	oldSession  = "session-token-1"
	otherLogin  = "session-token-2"
)

var accountTestNow = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

// accountRun は runCommand を、dataDir を MDM_DATA_DIR とする偽の外界で実行した結果である。
type accountRun struct {
	code   int
	stderr string
}

// runAccountCommand は args を stdin と共に実行する。hidden が nil でなければ、
// 標準入力を端末とみなし、エコーを止めた入力として hidden を順に返す。
func runAccountCommand(t *testing.T, dataDir string, args []string, stdin string, hidden []string) accountRun {
	t.Helper()
	var stderr bytes.Buffer
	env := accountEnv{
		Getenv: envFrom(map[string]string{envDataDir: dataDir}),
		Stdin:  strings.NewReader(stdin),
		Stderr: &stderr,
		Now:    func() time.Time { return accountTestNow.Add(time.Hour) },
	}
	if hidden != nil {
		env.ReadHidden = func() ([]byte, error) {
			if len(hidden) == 0 {
				return nil, errors.New("入力がありません")
			}
			next := hidden[0]
			hidden = hidden[1:]
			return []byte(next), nil
		}
	}
	code := runCommand(context.Background(), args, env)
	return accountRun{code: code, stderr: stderr.String()}
}

// newMigratedDataDir はマイグレーション済みで、アカウントの無いデータディレクトリを作る。
func newMigratedDataDir(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	db, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := store.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return dataDir
}

// newConfiguredDataDir は、初回設定と別のログインでセッションが2つあるデータディレクトリを作る。
func newConfiguredDataDir(t *testing.T) string {
	t.Helper()
	dataDir := newMigratedDataDir(t)
	withAuth(t, dataDir, func(auth *store.AuthStore) {
		hash, err := password.Hash(oldPassword)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if err := auth.Setup(ctx, oldUsername, hash, oldSession, accountTestNow); err != nil {
			t.Fatal(err)
		}
		if err := auth.AddSession(ctx, otherLogin, 1, "", accountTestNow); err != nil {
			t.Fatal(err)
		}
	})
	return dataDir
}

func withAuth(t *testing.T, dataDir string, use func(*store.AuthStore)) {
	t.Helper()
	db, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	use(db.Auth())
}

func readAccount(t *testing.T, dataDir string) domain.Account {
	t.Helper()
	var account domain.Account
	withAuth(t, dataDir, func(auth *store.AuthStore) {
		var err error
		account, err = auth.Account(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	})
	return account
}

func sessionValid(t *testing.T, dataDir, token string) bool {
	t.Helper()
	var valid bool
	withAuth(t, dataDir, func(auth *store.AuthStore) {
		var err error
		_, valid, err = auth.Session(context.Background(), token, accountTestNow.Add(2*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
	})
	return valid
}

// countRows は table の行数を返す。保存層の型には件数を数える操作が無いので、
// データベースのファイルを直接読む。
func countRows(t *testing.T, dataDir, table string) int {
	t.Helper()
	handle, err := sql.Open("sqlite", store.DatabasePath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = handle.Close() }()
	var count int
	if err := handle.QueryRow(`select count(*) from ` + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// credentialsMatch はログインの照合と同じく、ユーザー名をバイト列で比べ、パスワードを
// 保存されたハッシュで照合する。
func credentialsMatch(t *testing.T, account domain.Account, username, secret string) bool {
	t.Helper()
	ok, err := password.Verify(secret, account.PasswordHash)
	if err != nil {
		t.Fatal(err)
	}
	return ok && username == account.Username
}

// assertNoPlaintext は、データディレクトリのどのファイルにも出力にも secret が無いことを確かめる。
func assertNoPlaintext(t *testing.T, dataDir, secret string, outputs ...string) {
	t.Helper()
	for _, output := range outputs {
		if strings.Contains(output, secret) {
			t.Fatalf("出力に平文のパスワードがある: %q", output)
		}
	}
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dataDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(content, []byte(secret)) {
			t.Fatalf("%s に平文のパスワードがある", entry.Name())
		}
	}
}

func TestAccountCommandsChangeCredentialsAndInvalidateSessions(t *testing.T) {
	// ffmpeg・ffprobe が無いホストでも動く（起動前確認を行わない）。
	t.Setenv("PATH", "")
	dataDir := newConfiguredDataDir(t)
	if !sessionValid(t, dataDir, oldSession) || !sessionValid(t, dataDir, otherLogin) {
		t.Fatal("用意したセッションが有効でない")
	}

	renamed := runAccountCommand(t, dataDir, []string{"account", "set-username", newUsername}, "", nil)
	if renamed.code != exitAccountOK {
		t.Fatalf("set-username の終了コード = %d, 標準エラー = %q", renamed.code, renamed.stderr)
	}
	if !strings.Contains(renamed.stderr, "ユーザー名を変更しました") ||
		!strings.Contains(renamed.stderr, "セッションはすべて無効にしました") {
		t.Fatalf("標準エラー = %q", renamed.stderr)
	}
	if sessionValid(t, dataDir, oldSession) || sessionValid(t, dataDir, otherLogin) {
		t.Fatal("ユーザー名を変えた後も既存のセッションが有効")
	}
	if got := countRows(t, dataDir, "sessions"); got != 0 {
		t.Fatalf("sessions の行数 = %d", got)
	}

	// ユーザー名を変えた後に作ったセッションも、パスワードの再設定で無効になる。
	withAuth(t, dataDir, func(auth *store.AuthStore) {
		account, err := auth.Account(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if err := auth.AddSession(context.Background(), "session-token-3", account.Version, "", accountTestNow); err != nil {
			t.Fatal(err)
		}
	})
	if !sessionValid(t, dataDir, "session-token-3") {
		t.Fatal("用意したセッションが有効でない")
	}

	reset := runAccountCommand(t, dataDir, []string{"account", "set-password"}, newPassword+"\n", nil)
	if reset.code != exitAccountOK {
		t.Fatalf("set-password の終了コード = %d, 標準エラー = %q", reset.code, reset.stderr)
	}
	if !strings.Contains(reset.stderr, "パスワードを再設定しました") ||
		!strings.Contains(reset.stderr, "セッションはすべて無効にしました") {
		t.Fatalf("標準エラー = %q", reset.stderr)
	}
	if sessionValid(t, dataDir, "session-token-3") {
		t.Fatal("パスワードを再設定した後も既存のセッションが有効")
	}

	account := readAccount(t, dataDir)
	if strings.Contains(reset.stderr, account.PasswordHash) {
		t.Fatal("標準エラーにパスワードのハッシュがある")
	}
	for _, tc := range []struct {
		username, secret string
		want             bool
	}{
		{newUsername, newPassword, true},
		{oldUsername, newPassword, false},
		{newUsername, oldPassword, false},
		{oldUsername, oldPassword, false},
	} {
		if got := credentialsMatch(t, account, tc.username, tc.secret); got != tc.want {
			t.Errorf("照合(%q, %q) = %v, want %v", tc.username, tc.secret, got, tc.want)
		}
	}
	assertNoPlaintext(t, dataDir, newPassword, renamed.stderr, reset.stderr)
}

func TestAccountSetPasswordReadsFirstLineWithoutNewline(t *testing.T) {
	for name, stdin := range map[string]string{
		"改行なし":    newPassword,
		"CRLF":    newPassword + "\r\n2行目は使わない\n",
		"LF で終わる": newPassword + "\nnext",
	} {
		t.Run(name, func(t *testing.T) {
			dataDir := newConfiguredDataDir(t)
			run := runAccountCommand(t, dataDir, []string{"account", "set-password"}, stdin, nil)
			if run.code != exitAccountOK {
				t.Fatalf("終了コード = %d, 標準エラー = %q", run.code, run.stderr)
			}
			if !credentialsMatch(t, readAccount(t, dataDir), oldUsername, newPassword) {
				t.Fatal("標準入力の最初の1行で照合できない")
			}
		})
	}
}

func TestAccountSetPasswordKeepsTrailingCRWithoutNewline(t *testing.T) {
	dataDir := newConfiguredDataDir(t)
	withCR := newPassword + "\r"
	run := runAccountCommand(t, dataDir, []string{"account", "set-password"}, withCR, nil)
	if run.code != exitAccountOK {
		t.Fatalf("終了コード = %d, 標準エラー = %q", run.code, run.stderr)
	}
	account := readAccount(t, dataDir)
	if !credentialsMatch(t, account, oldUsername, withCR) {
		t.Fatal("改行なしで終わる入力の末尾の CR が失われた")
	}
	if credentialsMatch(t, account, oldUsername, newPassword) {
		t.Fatal("末尾の CR を除いた値で照合できてしまう")
	}
}

func TestAccountSetPasswordOnTerminalAsksTwice(t *testing.T) {
	dataDir := newConfiguredDataDir(t)

	mismatch := runAccountCommand(t, dataDir, []string{"account", "set-password"}, "",
		[]string{newPassword, newPassword + "x"})
	if mismatch.code != exitAccountUsage || !strings.Contains(mismatch.stderr, "一致しません") {
		t.Fatalf("不一致: 終了コード = %d, 標準エラー = %q", mismatch.code, mismatch.stderr)
	}
	if !credentialsMatch(t, readAccount(t, dataDir), oldUsername, oldPassword) || !sessionValid(t, dataDir, oldSession) {
		t.Fatal("確認が一致しないのに書き換えた")
	}

	matched := runAccountCommand(t, dataDir, []string{"account", "set-password"}, "", []string{newPassword, newPassword})
	if matched.code != exitAccountOK {
		t.Fatalf("一致: 終了コード = %d, 標準エラー = %q", matched.code, matched.stderr)
	}
	if !credentialsMatch(t, readAccount(t, dataDir), oldUsername, newPassword) {
		t.Fatal("端末から入力したパスワードで照合できない")
	}
	assertNoPlaintext(t, dataDir, newPassword, mismatch.stderr, matched.stderr)
}

func TestAccountCommandsRejectWithoutWriting(t *testing.T) {
	tooLong := strings.Repeat("a", domain.MaxPasswordBytes+1)
	cases := []struct {
		name       string
		configured bool
		args       []string
		stdin      string
		wantInErr  string
	}{
		{"未設定で set-username", false, []string{"account", "set-username", newUsername}, "", "初回設定"},
		{"未設定で set-password", false, []string{"account", "set-password"}, newPassword + "\n", "初回設定"},
		{"空のユーザー名", true, []string{"account", "set-username", ""}, "", "ユーザー名が正しくありません"},
		{"前後に空白のあるユーザー名", true, []string{"account", "set-username", " bob"}, "", "ユーザー名が正しくありません"},
		{"ユーザー名が無い", true, []string{"account", "set-username"}, "", "1つだけ"},
		{"空のパスワード", true, []string{"account", "set-password"}, "\n", "パスワードが正しくありません"},
		{"長すぎるパスワード", true, []string{"account", "set-password"}, tooLong + "\n", "パスワードが正しくありません"},
		{"パスワードを引数で渡す", true, []string{"account", "set-password", newPassword}, "", "引数を取りません"},
		{"未知の下位コマンド", true, []string{"account", "delete"}, "", "未知の下位コマンド"},
		{"下位コマンドが無い", true, []string{"account"}, "", "下位コマンドを指定"},
		{"未知のコマンド", true, []string{"serve"}, "", "未知のコマンド"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var dataDir string
			if tc.configured {
				dataDir = newConfiguredDataDir(t)
			} else {
				dataDir = newMigratedDataDir(t)
			}
			run := runAccountCommand(t, dataDir, tc.args, tc.stdin, nil)
			if run.code != exitAccountUsage {
				t.Fatalf("終了コード = %d, 標準エラー = %q", run.code, run.stderr)
			}
			if !strings.Contains(run.stderr, tc.wantInErr) || !strings.Contains(run.stderr, "使い方") {
				t.Fatalf("標準エラー = %q", run.stderr)
			}
			if strings.Contains(run.stderr, newPassword) {
				t.Fatal("標準エラーに平文のパスワードがある")
			}

			if !tc.configured {
				if accounts, sessions := countRows(t, dataDir, "account"), countRows(t, dataDir, "sessions"); accounts != 0 || sessions != 0 {
					t.Fatalf("account = %d 行, sessions = %d 行", accounts, sessions)
				}
				return
			}
			account := readAccount(t, dataDir)
			if account.Version != 1 || !credentialsMatch(t, account, oldUsername, oldPassword) {
				t.Fatalf("アカウントが書き換わった: %+v", account)
			}
			if countRows(t, dataDir, "sessions") != 2 || !sessionValid(t, dataDir, oldSession) {
				t.Fatal("セッションが書き換わった")
			}
		})
	}
}

func TestAccountCommandDoesNotCreateMissingDatabase(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "typo")
	run := runAccountCommand(t, dataDir, []string{"account", "set-password"}, newPassword+"\n", nil)
	if run.code != exitAccountUsage || !strings.Contains(run.stderr, "初回設定") {
		t.Fatalf("終了コード = %d, 標準エラー = %q", run.code, run.stderr)
	}
	if _, err := os.Stat(dataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("データディレクトリが作られた: %v", err)
	}
}

func TestAccountCommandReportsUnopenableDatabase(t *testing.T) {
	dataDir := t.TempDir()
	// データベースのパスがディレクトリなので、SQLite として開けない。
	if err := os.Mkdir(store.DatabasePath(dataDir), 0o755); err != nil {
		t.Fatal(err)
	}
	run := runAccountCommand(t, dataDir, []string{"account", "set-username", newUsername}, "", nil)
	if run.code != exitAccountError {
		t.Fatalf("終了コード = %d, 標準エラー = %q", run.code, run.stderr)
	}
}

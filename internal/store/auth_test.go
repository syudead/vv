package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

var authNow = time.Unix(1_800_000_000, 0)

func countSessions(t *testing.T, db *DB) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from sessions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func sessionValid(t *testing.T, auth *AuthStore, token string, now time.Time) bool {
	t.Helper()
	_, ok, err := auth.Session(context.Background(), token, now)
	if err != nil {
		t.Fatalf("Session(%q): %v", token, err)
	}
	return ok
}

func TestAuthStoreAccountNotConfiguredWithoutRow(t *testing.T) {
	db := migratedDB(t)
	if _, err := db.Auth().Account(context.Background()); !errors.Is(err, domain.ErrAccountNotConfigured) {
		t.Fatalf("Account() err = %v, want ErrAccountNotConfigured", err)
	}
}

func TestAuthStoreSetupOnlyOnce(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	auth := db.Auth()

	if err := auth.Setup(ctx, "Owner", "hash-1", "token-1", authNow); err != nil {
		t.Fatal(err)
	}
	account, err := auth.Account(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if account != (domain.Account{Username: "Owner", PasswordHash: "hash-1", Version: 1}) {
		t.Fatalf("Account() = %+v", account)
	}
	expires, ok, err := auth.Session(ctx, "token-1", authNow)
	if err != nil || !ok {
		t.Fatalf("初回設定のセッションが有効でない: ok=%v err=%v", ok, err)
	}
	if want := authNow.Add(domain.SessionLifetime); !expires.Equal(want) {
		t.Errorf("期限 = %v, want %v", expires, want)
	}

	err = auth.Setup(ctx, "other", "hash-2", "token-2", authNow)
	if !errors.Is(err, domain.ErrAccountAlreadyConfigured) {
		t.Fatalf("2回目の Setup err = %v, want ErrAccountAlreadyConfigured", err)
	}
	account, err = auth.Account(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if account.Username != "Owner" || account.PasswordHash != "hash-1" {
		t.Errorf("2回目の Setup が書き換えた: %+v", account)
	}
	if got := countSessions(t, db); got != 1 {
		t.Errorf("sessions = %d, want 1（2回目はセッションも書かない）", got)
	}
	if sessionValid(t, auth, "token-2", authNow) {
		t.Error("失敗した初回設定のセッションが有効")
	}
}

func TestAuthStoreConcurrentSetupOneWins(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	auth := db.Auth()

	const n = 4
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			errs[i] = auth.Setup(ctx, "user", "hash", "token-"+string(rune('a'+i)), authNow)
		})
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrAccountAlreadyConfigured):
		default:
			t.Fatalf("予期しない誤り: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("成功した初回設定 = %d, want 1", succeeded)
	}
	if got := countSessions(t, db); got != 1 {
		t.Errorf("sessions = %d, want 1", got)
	}
}

func TestAuthStoreChangeCredentialsRequiresAccount(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if err := db.Auth().ChangeUsername(ctx, "u", authNow); !errors.Is(err, domain.ErrAccountNotConfigured) {
		t.Errorf("ChangeUsername err = %v, want ErrAccountNotConfigured", err)
	}
	if err := db.Auth().ChangePassword(ctx, "h", authNow); !errors.Is(err, domain.ErrAccountNotConfigured) {
		t.Errorf("ChangePassword err = %v, want ErrAccountNotConfigured", err)
	}
	var count int
	if err := db.sql.QueryRow(`select count(*) from account`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("account = %d 行, want 0", count)
	}
}

func TestAuthStoreChangeCredentialsInvalidatesSessions(t *testing.T) {
	for name, change := range map[string]func(*AuthStore) error{
		"username": func(a *AuthStore) error { return a.ChangeUsername(context.Background(), "renamed", authNow) },
		"password": func(a *AuthStore) error { return a.ChangePassword(context.Background(), "hash-new", authNow) },
	} {
		t.Run(name, func(t *testing.T) {
			db := migratedDB(t)
			ctx := context.Background()
			auth := db.Auth()
			if err := auth.Setup(ctx, "owner", "hash", "setup-token", authNow); err != nil {
				t.Fatal(err)
			}
			before, err := auth.Account(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := auth.AddSession(ctx, "login-token", before.Version, "", authNow); err != nil {
				t.Fatal(err)
			}

			if err := change(auth); err != nil {
				t.Fatal(err)
			}
			after, err := auth.Account(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if after.Version != before.Version+1 {
				t.Errorf("Version = %d, want %d", after.Version, before.Version+1)
			}
			for _, token := range []string{"setup-token", "login-token"} {
				if sessionValid(t, auth, token, authNow) {
					t.Errorf("書き換え前の %s が有効", token)
				}
			}
			if got := countSessions(t, db); got != 0 {
				t.Errorf("sessions = %d, want 0", got)
			}

			// 書き換え前に読んだ版で照合したログインが、書き換えの後に行を足しても無効。
			if err := auth.AddSession(ctx, "stale-token", before.Version, "", authNow); err != nil {
				t.Fatal(err)
			}
			if sessionValid(t, auth, "stale-token", authNow) {
				t.Error("古い版で足したセッションが有効")
			}
			if err := auth.AddSession(ctx, "fresh-token", after.Version, "", authNow); err != nil {
				t.Fatal(err)
			}
			if !sessionValid(t, auth, "fresh-token", authNow) {
				t.Error("今の版で足したセッションが無効")
			}
		})
	}
}

func TestAuthStoreChangeUsernameAndPasswordWriteValues(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	auth := db.Auth()
	if err := auth.Setup(ctx, "owner", "hash", "t", authNow); err != nil {
		t.Fatal(err)
	}
	if err := auth.ChangeUsername(ctx, "Owner2", authNow); err != nil {
		t.Fatal(err)
	}
	if err := auth.ChangePassword(ctx, "hash-2", authNow); err != nil {
		t.Fatal(err)
	}
	account, err := auth.Account(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if account != (domain.Account{Username: "Owner2", PasswordHash: "hash-2", Version: 3}) {
		t.Errorf("Account() = %+v", account)
	}
}

func TestAuthStoreExpiredSessions(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	auth := db.Auth()
	if err := auth.Setup(ctx, "owner", "hash", "old", authNow); err != nil {
		t.Fatal(err)
	}
	expiry := authNow.Add(domain.SessionLifetime)

	if !sessionValid(t, auth, "old", expiry.Add(-time.Second)) {
		t.Error("期限の直前に無効")
	}
	// 使っても延長しない。
	if sessionValid(t, auth, "old", expiry) {
		t.Error("期限ちょうどで有効")
	}
	if got := countSessions(t, db); got != 0 {
		t.Errorf("確かめた後の sessions = %d, want 0（期限切れの行は消える）", got)
	}

	// 起動時の掃除。
	for _, token := range []string{"a", "b"} {
		if err := auth.AddSession(ctx, token, 1, "", authNow); err != nil {
			t.Fatal(err)
		}
	}
	if err := auth.AddSession(ctx, "later", 1, "", authNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	deleted, err := auth.DeleteExpiredSessions(ctx, expiry)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Errorf("掃除で消えた行 = %d, want 2", deleted)
	}
	if !sessionValid(t, auth, "later", expiry) {
		t.Error("期限前のセッションが掃除で消えた")
	}
}

func TestAuthStoreAddSessionCleansExpiredAndReplaced(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	auth := db.Auth()
	if err := auth.Setup(ctx, "owner", "hash", "expired", authNow); err != nil {
		t.Fatal(err)
	}
	later := authNow.Add(domain.SessionLifetime)
	if err := auth.AddSession(ctx, "current", 1, "", later.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	if err := auth.AddSession(ctx, "next", 1, "current", later); err != nil {
		t.Fatal(err)
	}
	if got := countSessions(t, db); got != 1 {
		t.Errorf("sessions = %d, want 1（期限切れと置き換えた行が消える）", got)
	}
	if sessionValid(t, auth, "current", later) {
		t.Error("置き換えた古いセッションが有効")
	}
	if !sessionValid(t, auth, "next", later) {
		t.Error("新しいセッションが無効")
	}
}

func TestAuthStoreDeleteSession(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	auth := db.Auth()
	if err := auth.Setup(ctx, "owner", "hash", "tok", authNow); err != nil {
		t.Fatal(err)
	}
	if err := auth.DeleteSession(ctx, "tok"); err != nil {
		t.Fatal(err)
	}
	if sessionValid(t, auth, "tok", authNow) {
		t.Error("ログアウトしたセッションが有効")
	}
	if err := auth.DeleteSession(ctx, "missing"); err != nil {
		t.Errorf("無いセッションの削除が誤り: %v", err)
	}
}

func TestAuthStoreUnknownSessionIsInvalid(t *testing.T) {
	db := migratedDB(t)
	if sessionValid(t, db.Auth(), "nothing", authNow) {
		t.Error("無いセッションが有効")
	}
}

// セッション ID そのものは保存せず、SHA-256 の16進だけを置く。
func TestAuthStoreStoresOnlyTokenHash(t *testing.T) {
	db := migratedDB(t)
	if err := db.Auth().Setup(context.Background(), "owner", "hash", "secret-token", authNow); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.sql.QueryRow(`select token_hash from sessions`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "secret-token") || stored != sessionTokenHash("secret-token") || len(stored) != 64 {
		t.Errorf("token_hash = %q", stored)
	}
}

func TestAuthStoreSessionQueryFailureIsError(t *testing.T) {
	db := migratedDB(t)
	if _, err := db.sql.Exec(`drop table sessions`); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := db.Auth().Session(context.Background(), "tok", authNow); err == nil || ok {
		t.Errorf("Session() = ok=%v err=%v, want error", ok, err)
	}
}

func TestAuthStoreUsernameIsCaseSensitive(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if err := db.Auth().Setup(ctx, "Alice", "hash", "t", authNow); err != nil {
		t.Fatal(err)
	}
	account, err := db.Auth().Account(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if account.Username != "Alice" {
		t.Errorf("Username = %q, want Alice", account.Username)
	}
	var matches int
	if err := db.sql.QueryRow(`select count(*) from account where username = 'alice'`).Scan(&matches); err != nil {
		t.Fatal(err)
	}
	if matches != 0 {
		t.Error("username の照合が大文字小文字を区別しない")
	}
}

func TestAuthMigrationDownDropsTables(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if err := db.Auth().Setup(ctx, "owner", "hash", "t", authNow); err != nil {
		t.Fatal(err)
	}
	downTo(t, db, 8)
	for _, name := range []string{"account", "sessions", "sessions_expires_at"} {
		var count int
		if err := db.sql.QueryRow(`select count(*) from sqlite_master where name = ?`, name).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s が残っている", name)
		}
	}
}

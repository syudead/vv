package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// setupSession は setupAccount が作るセッションの平文である。
const setupSession = "session"

func setupAccount(t *testing.T, db *DB) {
	t.Helper()
	if err := db.Auth().Setup(context.Background(), "owner", "hash", setupSession, authNow); err != nil {
		t.Fatal(err)
	}
}

func addAPIToken(t *testing.T, db *DB, name, token string, now time.Time) domain.APIToken {
	t.Helper()
	added, err := db.Auth().AddAPIToken(context.Background(), setupSession, name, token, now)
	if err != nil {
		t.Fatalf("AddAPIToken(%q): %v", name, err)
	}
	return added
}

func apiTokenValid(t *testing.T, db *DB, token string) bool {
	t.Helper()
	_, ok, err := db.Auth().APIToken(context.Background(), token)
	if err != nil {
		t.Fatalf("APIToken(%q): %v", token, err)
	}
	return ok
}

func TestAPITokenRequiresAccount(t *testing.T) {
	db := migratedDB(t)
	if _, err := db.Auth().AddAPIToken(context.Background(), setupSession, "n", "t", authNow); !errors.Is(err, domain.ErrSessionNotValid) {
		t.Fatalf("AddAPIToken err = %v, want ErrSessionNotValid", err)
	}
}

// 発行は、求めたセッションがその取引の中で有効なときだけ行う。境界で確かめた後に資格情報が
// 変わっていれば、新しい版で足さない（research.md R-2）。
func TestAPITokenRequiresCurrentSession(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(*testing.T, *DB)
		session string
		now     time.Time
	}{
		{name: "資格情報の変更の後", session: setupSession, now: authNow, prepare: func(t *testing.T, db *DB) {
			if err := db.Auth().ChangePassword(context.Background(), "new-hash", authNow); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "版の合わないセッション", session: setupSession, now: authNow, prepare: func(t *testing.T, db *DB) {
			if _, err := db.sql.Exec(`update account set version = version + 1`); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "期限切れ", session: setupSession, now: authNow.Add(domain.SessionLifetime)},
		{name: "無いセッション", session: "other", now: authNow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := migratedDB(t)
			setupAccount(t, db)
			if tc.prepare != nil {
				tc.prepare(t, db)
			}
			if _, err := db.Auth().AddAPIToken(context.Background(), tc.session, "n", "t1", tc.now); !errors.Is(err, domain.ErrSessionNotValid) {
				t.Fatalf("AddAPIToken err = %v, want ErrSessionNotValid", err)
			}
			var count int
			if err := db.sql.QueryRow(`select count(*) from api_tokens`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Errorf("api_tokens = %d 行, want 0", count)
			}
		})
	}
}

func TestAPITokenStoresOnlyHash(t *testing.T) {
	db := migratedDB(t)
	setupAccount(t, db)
	added := addAPIToken(t, db, "Claude", "vvt_secret", authNow)
	if added.ID == 0 || added.Name != "Claude" || !added.CreatedAt.Equal(authNow) || !added.LastUsedAt.IsZero() {
		t.Fatalf("AddAPIToken = %+v", added)
	}
	var hash string
	var version int64
	if err := db.sql.QueryRow(`select token_hash, account_version from api_tokens`).Scan(&hash, &version); err != nil {
		t.Fatal(err)
	}
	if hash == "vvt_secret" || hash != sessionTokenHash("vvt_secret") {
		t.Errorf("token_hash = %q", hash)
	}
	if version != 1 {
		t.Errorf("account_version = %d, want 1", version)
	}

	got, ok, err := db.Auth().APIToken(context.Background(), "vvt_secret")
	if err != nil || !ok || got != added {
		t.Fatalf("APIToken = %+v, %v, %v", got, ok, err)
	}
	if apiTokenValid(t, db, "vvt_other") {
		t.Error("発行していないトークンが有効")
	}
}

func TestAPITokenListOrderAndRevoke(t *testing.T) {
	db := migratedDB(t)
	setupAccount(t, db)
	ctx := context.Background()
	first := addAPIToken(t, db, "same", "t1", authNow)
	second := addAPIToken(t, db, "same", "t2", authNow.Add(time.Minute))
	third := addAPIToken(t, db, "other", "t3", authNow.Add(time.Minute))

	list, err := db.Auth().ListAPITokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].ID != third.ID || list[1].ID != second.ID || list[2].ID != first.ID {
		t.Fatalf("ListAPITokens = %+v", list)
	}

	if err := db.Auth().DeleteAPIToken(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	// 無い id は何もしない。
	if err := db.Auth().DeleteAPIToken(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if apiTokenValid(t, db, "t2") || !apiTokenValid(t, db, "t1") {
		t.Error("失効が別のトークンに及んだか、失効したトークンが有効")
	}
	// 失効した行の id は再利用しない。
	fourth := addAPIToken(t, db, "new", "t4", authNow.Add(2*time.Minute))
	if fourth.ID <= third.ID {
		t.Errorf("新しい id = %d, 以前の最大 = %d", fourth.ID, third.ID)
	}
	list, err = db.Auth().ListAPITokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("ListAPITokens = %+v", list)
	}
}

func TestAPITokenEmptyListIsNotNil(t *testing.T) {
	db := migratedDB(t)
	list, err := db.Auth().ListAPITokens(context.Background())
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("ListAPITokens = %#v, %v", list, err)
	}
}

func TestAPITokenChangeCredentialsDeletesTokens(t *testing.T) {
	for _, change := range []struct {
		name string
		run  func(*AuthStore) error
	}{
		{"username", func(a *AuthStore) error { return a.ChangeUsername(context.Background(), "new", authNow) }},
		{"password", func(a *AuthStore) error { return a.ChangePassword(context.Background(), "new-hash", authNow) }},
	} {
		t.Run(change.name, func(t *testing.T) {
			db := migratedDB(t)
			setupAccount(t, db)
			addAPIToken(t, db, "a", "t1", authNow)
			if err := change.run(db.Auth()); err != nil {
				t.Fatal(err)
			}
			if apiTokenValid(t, db, "t1") {
				t.Error("資格情報の変更の後もトークンが有効")
			}
			var count int
			if err := db.sql.QueryRow(`select count(*) from api_tokens`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Errorf("api_tokens = %d 行, want 0", count)
			}
		})
	}
}

// 版の合わない行は、残っていても一覧に出さず、有効としない（research.md R-2）。
func TestAPITokenStaleVersionIsInvalid(t *testing.T) {
	db := migratedDB(t)
	setupAccount(t, db)
	addAPIToken(t, db, "a", "t1", authNow)
	if _, err := db.sql.Exec(`update account set version = version + 1`); err != nil {
		t.Fatal(err)
	}
	if apiTokenValid(t, db, "t1") {
		t.Error("版の合わないトークンが有効")
	}
	list, err := db.Auth().ListAPITokens(context.Background())
	if err != nil || len(list) != 0 {
		t.Fatalf("ListAPITokens = %+v, %v", list, err)
	}
}

func TestAPITokenTouchWritesAtMostOncePerMinute(t *testing.T) {
	db := migratedDB(t)
	setupAccount(t, db)
	ctx := context.Background()
	added := addAPIToken(t, db, "a", "t1", authNow)
	lastUsed := func() time.Time {
		t.Helper()
		got, ok, err := db.Auth().APIToken(ctx, "t1")
		if err != nil || !ok {
			t.Fatalf("APIToken: %v, %v", ok, err)
		}
		return got.LastUsedAt
	}

	for _, step := range []struct {
		at   time.Duration
		want time.Duration
	}{
		{at: 10 * time.Second, want: 10 * time.Second},
		{at: 69 * time.Second, want: 10 * time.Second},
		{at: 70 * time.Second, want: 70 * time.Second},
	} {
		if err := db.Auth().TouchAPIToken(ctx, added.ID, authNow.Add(step.at)); err != nil {
			t.Fatal(err)
		}
		if got := lastUsed(); !got.Equal(authNow.Add(step.want)) {
			t.Errorf("%v に使った後の LastUsedAt = %v, want %v", step.at, got, authNow.Add(step.want))
		}
	}
	// 無い id は何もしない。
	if err := db.Auth().TouchAPIToken(ctx, added.ID+100, authNow); err != nil {
		t.Fatal(err)
	}
}

func TestAPITokenQueryFailureIsError(t *testing.T) {
	db := migratedDB(t)
	setupAccount(t, db)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok, err := db.Auth().APIToken(ctx, "t1"); err == nil || ok {
		t.Fatalf("APIToken = %v, %v, want 誤り", ok, err)
	}
}

func TestAPITokenMigrationDownDropsTable(t *testing.T) {
	db := migratedDB(t)
	downTo(t, db, 19)
	var count int
	if err := db.sql.QueryRow(`select count(*) from sqlite_master where name = 'api_tokens'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Error("api_tokens が残っている")
	}
}

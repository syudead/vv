package store

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

const (
	testGrantSecret = "vvp_incoming-secret-from-the-reader"
	testNodeID      = "0b6d8c1e-5c55-4d1a-9a59-6f0b1e2a3c4d"
	otherNodeID     = "7c2f4c3a-1d2e-4f50-8a6b-0c1d2e3f4a5b"
)

func testLink(nodeID string) domain.PeerLink {
	return domain.PeerLink{
		NodeID: nodeID, Name: "nas", Address: "http://192.168.1.20:8080", Secret: "vvp_outgoing", PairedAt: authNow,
	}
}

func testGrant(nodeID string) domain.PeerGrant {
	return domain.PeerGrant{NodeID: nodeID, Name: "den", PairedAt: authNow}
}

func addTestLink(t *testing.T, db *DB) {
	t.Helper()
	if err := db.Peers().AddLink(context.Background(), testLink(testNodeID)); err != nil {
		t.Fatalf("AddLink: %v", err)
	}
}

func addTestGrant(t *testing.T, db *DB) {
	t.Helper()
	if err := db.Peers().AddGrant(context.Background(), testGrant(testNodeID), testGrantSecret); err != nil {
		t.Fatalf("AddGrant: %v", err)
	}
}

// Each direction holds one row, and the two directions are independent: the same node can be both.
func TestPeerStoreKeepsOneLinkAndOneGrant(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	addTestLink(t, db)
	addTestGrant(t, db)

	if err := db.Peers().AddLink(ctx, testLink(otherNodeID)); !errors.Is(err, domain.ErrAlreadyPaired) {
		t.Errorf("2 つ目の AddLink err = %v, want ErrAlreadyPaired", err)
	}
	if err := db.Peers().AddGrant(ctx, testGrant(otherNodeID), "vvp_other"); !errors.Is(err, domain.ErrAlreadyPaired) {
		t.Errorf("2 つ目の AddGrant err = %v, want ErrAlreadyPaired", err)
	}
	if link, found, err := db.Peers().Link(ctx); err != nil || !found || link != testLink(testNodeID) {
		t.Errorf("Link = %+v, %v, %v, want %+v", link, found, err, testLink(testNodeID))
	}
	if grant, found, err := db.Peers().Grant(ctx); err != nil || !found || grant != testGrant(testNodeID) {
		t.Errorf("Grant = %+v, %v, %v, want %+v", grant, found, err, testGrant(testNodeID))
	}

	// Deleting one direction leaves the other.
	if deleted, err := db.Peers().DeleteGrant(ctx, otherNodeID); err != nil || deleted {
		t.Fatalf("別の node id の DeleteGrant = %v, %v, want 消さない", deleted, err)
	}
	if deleted, err := db.Peers().DeleteGrant(ctx, testNodeID); err != nil || !deleted {
		t.Fatalf("DeleteGrant = %v, %v", deleted, err)
	}
	if _, found, _ := db.Peers().Grant(ctx); found {
		t.Error("消した grant が残っている")
	}
	if _, found, _ := db.Peers().Link(ctx); !found {
		t.Error("grant を消すと link も消えた")
	}
}

func TestPeerStoreHashesTheGrantSecret(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	addTestGrant(t, db)

	var stored string
	if err := db.sql.QueryRow(`select secret_hash from peer_grant`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == testGrantSecret || stored != sessionTokenHash(testGrantSecret) {
		t.Errorf("secret_hash = %q, want SHA-256 の 16 進", stored)
	}
	if grant, found, err := db.Peers().GrantBySecret(ctx, testGrantSecret); err != nil || !found || grant.NodeID != testNodeID {
		t.Errorf("平文の GrantBySecret = %+v, %v, %v", grant, found, err)
	}
	for _, other := range []string{stored, "vvp_outgoing", ""} {
		if _, found, err := db.Peers().GrantBySecret(ctx, other); err != nil || found {
			t.Errorf("GrantBySecret(%q) = %v, %v, want 一致なし", other, found, err)
		}
	}
}

// Requirement 6 of #1629: a credential change keeps the pairing.
func TestPeerStoreSurvivesACredentialChange(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	setupAccount(t, db)
	addTestGrant(t, db)

	if err := db.Auth().ChangePassword(ctx, "new-hash", authNow); err != nil {
		t.Fatal(err)
	}
	if _, found, err := db.Peers().GrantBySecret(ctx, testGrantSecret); err != nil || !found {
		t.Errorf("パスワード変更の後の GrantBySecret = %v, %v, want 一致", found, err)
	}
}

func TestDeleteLinkLeavesOtherDataAlone(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	videoID := looseVideo(t, db, "a")
	tag, err := db.Tags().CreateTag(ctx, "海")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{videoID}, tag.ID); err != nil {
		t.Fatal(err)
	}
	nodeID, err := db.Peers().EnsureNodeID(ctx, testNodeID)
	if err != nil {
		t.Fatal(err)
	}
	addTestLink(t, db)
	before := tableCounts(t, db)

	if deleted, err := db.Peers().DeleteLink(ctx, otherNodeID); err != nil || deleted {
		t.Fatalf("別の node id の DeleteLink = %v, %v, want 消さない", deleted, err)
	}
	if deleted, err := db.Peers().DeleteLink(ctx, testNodeID); err != nil || !deleted {
		t.Fatalf("DeleteLink = %v, %v", deleted, err)
	}

	after := tableCounts(t, db)
	if after["peer_link"] != 0 {
		t.Errorf("peer_link の行数 = %d, want 0", after["peer_link"])
	}
	delete(before, "peer_link")
	delete(after, "peer_link")
	for table, count := range before {
		if after[table] != count {
			t.Errorf("%s の行数が %d から %d に変わった", table, count, after[table])
		}
	}
	if again, err := db.Peers().EnsureNodeID(ctx, "another"); err != nil || again != nodeID {
		t.Errorf("node id = %q, %v, want %q のまま", again, err, nodeID)
	}
}

func TestLinkUpdatesNeedALink(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if _, err := db.Peers().UpdateLinkAddress(ctx, "http://nas.local"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("UpdateLinkAddress err = %v, want ErrNotFound", err)
	}
	addTestLink(t, db)

	if err := db.Peers().PinLinkCertificate(ctx, testNodeID, "spki-confirmed"); err != nil {
		t.Fatal(err)
	}
	if err := db.Peers().MarkLinkRevoked(ctx, testNodeID); err != nil {
		t.Fatal(err)
	}
	link, err := db.Peers().UpdateLinkAddress(ctx, "http://nas.local")
	if err != nil || link.PinnedSPKI != "spki-confirmed" || !link.Revoked || link.Address != "http://nas.local" {
		t.Errorf("Link = %+v, %v, want 固定した指紋、revoked、新しいアドレス", link, err)
	}
	if err := db.Peers().PinLinkCertificate(ctx, otherNodeID, "spki-other"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("別の node id の PinLinkCertificate err = %v, want ErrNotFound", err)
	}
}

// A mutual pairing of the old schema keeps both directions; a record the peer had revoked keeps only a
// revoked link, so nobody is silently unpaired and nobody gains access.
func TestPeerMigrationSplitsTheMutualRecord(t *testing.T) {
	cases := []struct {
		name         string
		incomingHash any
		wantRevoked  bool
		wantGrant    bool
	}{
		{name: "相互のペアリング", incomingHash: sessionTokenHash(testGrantSecret), wantGrant: true},
		{name: "相手側で解除済み", incomingHash: nil, wantRevoked: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, provider := migrateTo(t, 82)
			ctx := context.Background()
			if _, err := db.sql.ExecContext(ctx, `
				insert into peer (node_id, name, address, outgoing_secret, incoming_secret_hash, pinned_spki, paired_at)
				values (?, 'nas', 'https://nas.local', 'vvp_outgoing', ?, 'spki', ?)`,
				testNodeID, tc.incomingHash, authNow.Unix()); err != nil {
				t.Fatal(err)
			}
			if _, err := provider.UpTo(ctx, 83); err != nil {
				t.Fatal(err)
			}

			link, found, err := db.Peers().Link(ctx)
			want := domain.PeerLink{
				NodeID: testNodeID, Name: "nas", Address: "https://nas.local", Secret: "vvp_outgoing",
				Revoked: tc.wantRevoked, PinnedSPKI: "spki", PairedAt: authNow,
			}
			if err != nil || !found || link != want {
				t.Errorf("Link = %+v, %v, %v, want %+v", link, found, err, want)
			}
			grant, found, err := db.Peers().GrantBySecret(ctx, testGrantSecret)
			if err != nil || found != tc.wantGrant || (found && grant != domain.PeerGrant{NodeID: testNodeID, Name: "nas", PairedAt: authNow}) {
				t.Errorf("GrantBySecret = %+v, %v, %v, want found %v", grant, found, err, tc.wantGrant)
			}
		})
	}
}

func tableCounts(t *testing.T, db *DB) map[string]int {
	t.Helper()
	rows, err := db.sql.Query(`select name from sqlite_master where type = 'table' and name not like 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, table := range tables {
		var count int
		if err := db.sql.QueryRow(`select count(*) from "` + table + `"`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		counts[table] = count
	}
	return counts
}

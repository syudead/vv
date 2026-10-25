package store

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

const (
	testIncomingSecret = "vvp_incoming-secret-from-the-peer"
	testNodeID         = "0b6d8c1e-5c55-4d1a-9a59-6f0b1e2a3c4d"
)

func testPeerRecord(nodeID string) domain.PeerRecord {
	return domain.PeerRecord{
		NodeID: nodeID, Name: "nas", Address: "http://192.168.1.20:8080",
		OutgoingSecret: "vvp_outgoing", PairedAt: authNow,
	}
}

func addTestPeer(t *testing.T, db *DB) {
	t.Helper()
	if err := db.Peers().AddPeer(context.Background(), testPeerRecord(testNodeID), testIncomingSecret); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
}

func TestPeerStoreKeepsOneRecord(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	addTestPeer(t, db)

	err := db.Peers().AddPeer(ctx, testPeerRecord("7c2f4c3a-1d2e-4f50-8a6b-0c1d2e3f4a5b"), "vvp_other")
	if !errors.Is(err, domain.ErrAlreadyPaired) {
		t.Fatalf("2 つ目の AddPeer err = %v, want ErrAlreadyPaired", err)
	}
	got, found, err := db.Peers().Peer(ctx)
	if err != nil || !found {
		t.Fatalf("Peer = %v, %v", found, err)
	}
	want := testPeerRecord(testNodeID)
	if got != want {
		t.Errorf("Peer = %+v, want %+v", got, want)
	}
}

func TestPeerStoreHashesTheIncomingSecret(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	addTestPeer(t, db)

	var stored string
	if err := db.sql.QueryRow(`select incoming_secret_hash from peer`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == testIncomingSecret || stored != sessionTokenHash(testIncomingSecret) {
		t.Errorf("incoming_secret_hash = %q, want SHA-256 の 16 進", stored)
	}
	if record, found, err := db.Peers().PeerBySecret(ctx, testIncomingSecret); err != nil || !found || record.NodeID != testNodeID {
		t.Errorf("平文の PeerBySecret = %+v, %v, %v", record, found, err)
	}
	for _, other := range []string{stored, "vvp_outgoing", ""} {
		if _, found, err := db.Peers().PeerBySecret(ctx, other); err != nil || found {
			t.Errorf("PeerBySecret(%q) = %v, %v, want 一致なし", other, found, err)
		}
	}

	if err := db.Peers().RevokeIncomingSecret(ctx, testNodeID); err != nil {
		t.Fatal(err)
	}
	if _, found, err := db.Peers().PeerBySecret(ctx, testIncomingSecret); err != nil || found {
		t.Errorf("取り消した後の PeerBySecret = %v, %v, want 一致なし", found, err)
	}
	if record, _, _ := db.Peers().Peer(ctx); !record.IncomingRevoked {
		t.Error("取り消した後の Peer の IncomingRevoked が false")
	}
}

// Requirement 6: a credential change keeps the pairing.
func TestPeerStoreSurvivesACredentialChange(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	setupAccount(t, db)
	addTestPeer(t, db)

	if err := db.Auth().ChangePassword(ctx, "new-hash", authNow); err != nil {
		t.Fatal(err)
	}
	if _, found, err := db.Peers().PeerBySecret(ctx, testIncomingSecret); err != nil || !found {
		t.Errorf("パスワード変更の後の PeerBySecret = %v, %v, want 一致", found, err)
	}
}

func TestDeletePeerLeavesOtherDataAlone(t *testing.T) {
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
	addTestPeer(t, db)
	before := tableCounts(t, db)

	if deleted, err := db.Peers().DeletePeer(ctx, "7c2f4c3a-1d2e-4f50-8a6b-0c1d2e3f4a5b"); err != nil || deleted {
		t.Fatalf("別の node id の DeletePeer = %v, %v, want 消さない", deleted, err)
	}
	if deleted, err := db.Peers().DeletePeer(ctx, testNodeID); err != nil || !deleted {
		t.Fatalf("DeletePeer = %v, %v", deleted, err)
	}

	after := tableCounts(t, db)
	if after["peer"] != 0 {
		t.Errorf("peer の行数 = %d, want 0", after["peer"])
	}
	delete(before, "peer")
	delete(after, "peer")
	for table, count := range before {
		if after[table] != count {
			t.Errorf("%s の行数が %d から %d に変わった", table, count, after[table])
		}
	}
	if again, err := db.Peers().EnsureNodeID(ctx, "another"); err != nil || again != nodeID {
		t.Errorf("node id = %q, %v, want %q のまま", again, err, nodeID)
	}
}

func TestUpdatePeerAddressWithoutPeer(t *testing.T) {
	db := migratedDB(t)
	if _, err := db.Peers().UpdatePeerAddress(context.Background(), "http://nas.local"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("UpdatePeerAddress err = %v, want ErrNotFound", err)
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

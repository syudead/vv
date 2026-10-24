package store

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Playback fixture: standalone A, group G with its first member watched, then standalone B.
type playbackFixture struct {
	db                  *DB
	id                  int64
	a, b                int64
	members             []int64
	itemA, itemG, itemB int64
}

func newPlaybackFixture(t *testing.T) playbackFixture {
	t.Helper()
	db, ids := itemsFixture(t)
	// Clear member three's in-progress record so member two is the group's opening choice.
	if _, err := db.sql.Exec(`delete from playback_progress where content_key = ?`,
		"key"+fixturePath("/media/show/ep10.mp4")); err != nil {
		t.Fatal(err)
	}
	f := playbackFixture{db: db, a: ids[fixturePath("/media/mixed/x.mp4")], b: ids[fixturePath("/media/mixed/sub/y.mp4")]}
	f.id = createPlaylist(t, db, "連続再生")
	addToPlaylist(t, db, f.id, domain.PlaylistAdd{VideoIDs: []int64{f.a}, GroupIDs: []int64{groupIDOf(t, db, fixturePath("/media/show"))}})
	addVideos(t, db, f.id, f.b)
	_, items := readPlaylist(t, db, f.id)
	if len(items) != 3 || items[1].Group == nil {
		t.Fatalf("項目 = %+v", items)
	}
	f.itemA, f.itemG, f.itemB = items[0].ID, items[1].ID, items[2].ID
	for _, member := range items[1].Group.Members {
		f.members = append(f.members, member.ID)
	}
	if len(f.members) != 3 || f.members[0] != ids[fixturePath("/media/show/ep1.mp4")] {
		t.Fatalf("メンバー = %v", f.members)
	}
	return f
}

func (f playbackFixture) playback(t *testing.T, itemID, videoID int64) domain.PlaylistPlayback {
	t.Helper()
	playback, err := f.db.Playlists().PlaylistPlayback(context.Background(), f.id, itemID, videoID)
	if err != nil {
		t.Fatalf("PlaylistPlayback(%d, %d): %v", itemID, videoID, err)
	}
	return playback
}

func stepOf(step *domain.PlaylistStep) [2]int64 {
	if step == nil {
		return [2]int64{}
	}
	return [2]int64{step.ItemID, step.Video.ID}
}

func TestPlaylistPlaybackFollowsTheOrder(t *testing.T) {
	f := newPlaybackFixture(t)
	m2, m3 := f.members[1], f.members[2]

	fromA := f.playback(t, f.itemA, f.a)
	if stepOf(fromA.Next) != [2]int64{f.itemG, m2} || stepOf(fromA.NextItem) != [2]int64{f.itemG, m2} {
		t.Errorf("A から next = %v, nextItem = %v, want メンバー 2", stepOf(fromA.Next), stepOf(fromA.NextItem))
	}
	if fromA.Position != 1 || fromA.Count != 3 || fromA.PreviousItem != nil {
		t.Errorf("A = %d / %d, previousItem = %v", fromA.Position, fromA.Count, stepOf(fromA.PreviousItem))
	}
	if fromA.Playlist.ID != f.id || fromA.Playlist.Name != "連続再生" || fromA.Playlist.ItemCount != 3 {
		t.Errorf("playlist = %+v", fromA.Playlist)
	}
	if got := f.playback(t, f.itemG, m2); stepOf(got.Next) != [2]int64{f.itemG, m3} || got.Position != 2 {
		t.Errorf("メンバー 2 から next = %v, position = %d", stepOf(got.Next), got.Position)
	}
	if got := f.playback(t, f.itemG, m3); stepOf(got.Next) != [2]int64{f.itemB, f.b} {
		t.Errorf("メンバー 3 から next = %v, want B", stepOf(got.Next))
	}
	fromB := f.playback(t, f.itemB, f.b)
	if fromB.Next != nil || fromB.Position != 3 || fromB.Count != 3 {
		t.Errorf("B から next = %v, %d / %d", stepOf(fromB.Next), fromB.Position, fromB.Count)
	}
	if stepOf(fromB.PreviousItem) != [2]int64{f.itemG, m2} || fromB.NextItem != nil {
		t.Errorf("B の前後 = %v・%v", stepOf(fromB.PreviousItem), stepOf(fromB.NextItem))
	}
}

func TestPlaylistPlaybackReadsTheLatestOrder(t *testing.T) {
	f := newPlaybackFixture(t)
	if err := f.db.Playlists().MovePlaylistItem(context.Background(), f.id, f.itemB, nil); err != nil {
		t.Fatal(err)
	}
	if got := f.playback(t, f.itemG, f.members[2]); got.Next != nil || got.Position != 3 {
		t.Errorf("動かしたあと メンバー 3 から next = %v, position = %d", stepOf(got.Next), got.Position)
	}
}

func TestPlaylistPlaybackSkipsAHiddenItem(t *testing.T) {
	f := newPlaybackFixture(t)
	if err := f.db.ScanIndex().DeleteVideos(context.Background(), []int64{f.a}); err != nil {
		t.Fatal(err)
	}
	got := f.playback(t, f.itemG, f.members[1])
	if got.Count != 2 || got.Position != 1 || got.PreviousItem != nil {
		t.Errorf("A が無いとき = %d / %d, previousItem = %v", got.Position, got.Count, stepOf(got.PreviousItem))
	}
}

func TestPlaylistPlaybackKeepsTheHiddenCurrentItemsPlace(t *testing.T) {
	f := newPlaybackFixture(t)
	if err := f.db.ScanIndex().DeleteVideos(context.Background(), f.members); err != nil {
		t.Fatal(err)
	}
	got := f.playback(t, f.itemG, f.members[1])
	if got.Position != 0 || got.Count != 2 {
		t.Errorf("隠れた今の項目 = %d / %d", got.Position, got.Count)
	}
	if stepOf(got.Next) != [2]int64{f.itemB, f.b} || stepOf(got.NextItem) != [2]int64{f.itemB, f.b} ||
		stepOf(got.PreviousItem) != [2]int64{f.itemA, f.a} {
		t.Errorf("next = %v, nextItem = %v, previousItem = %v", stepOf(got.Next), stepOf(got.NextItem), stepOf(got.PreviousItem))
	}
}

func TestPlaylistPlaybackStartsAWatchedGroupAtItsFirstMember(t *testing.T) {
	f := newPlaybackFixture(t)
	if _, err := f.db.sql.Exec(`insert into playback_progress (content_key, position_ms, duration_ms, completed, updated_at)
		select content_key, 1000, 1000, 1, 1 from videos where id in (?, ?, ?)
		on conflict (content_key) do update set completed = 1, position_ms = 1000`,
		f.members[0], f.members[1], f.members[2]); err != nil {
		t.Fatal(err)
	}
	got := f.playback(t, f.itemA, f.a)
	if stepOf(got.Next) != [2]int64{f.itemG, f.members[0]} || stepOf(got.NextItem) != [2]int64{f.itemG, f.members[0]} {
		t.Errorf("next = %v, nextItem = %v, want メンバー 1", stepOf(got.Next), stepOf(got.NextItem))
	}
}

// Item identity disambiguates a video appearing both standalone and within a group.
func TestPlaylistPlaybackUsesTheItemForAVideoInTwoItems(t *testing.T) {
	f := newPlaybackFixture(t)
	v := f.members[1]
	addVideos(t, f.db, f.id, v)
	if err := f.db.Playlists().MovePlaylistItem(context.Background(), f.id, f.itemB, &f.itemA); err != nil {
		t.Fatal(err)
	}
	ids := playlistItemIDs(t, f.db, f.id)
	itemV := ids[len(ids)-1]
	if len(ids) != 4 || ids[2] != f.itemG || itemV == f.itemG {
		t.Fatalf("項目 = %v", ids)
	}
	if err := f.db.Playlists().MovePlaylistItem(context.Background(), f.id, itemV, &f.itemA); err != nil {
		t.Fatal(err)
	}

	asMember := f.playback(t, f.itemG, v)
	asItem := f.playback(t, itemV, v)
	if stepOf(asMember.Next) != [2]int64{f.itemG, f.members[2]} {
		t.Errorf("G のメンバーとして next = %v, want G のメンバー 3", stepOf(asMember.Next))
	}
	if stepOf(asItem.Next) != [2]int64{f.itemB, f.b} {
		t.Errorf("V の項目として next = %v, want B", stepOf(asItem.Next))
	}
	if asMember.Position != 4 || asItem.Position != 2 {
		t.Errorf("position = %d・%d, want 4・2", asMember.Position, asItem.Position)
	}
}

// The store reads a virtual item with its virtual key, so continuous play passes over it while the manual step reaches it.
func TestPlaylistPlaybackPassesOverAVirtualItem(t *testing.T) {
	f := newPlaybackFixture(t)
	virtual := createVirtual(t, f.db, "Notes", fixedTime)
	addVideos(t, f.db, f.id, virtual.ID)
	ids := playlistItemIDs(t, f.db, f.id)
	itemV := ids[len(ids)-1]
	if err := f.db.Playlists().MovePlaylistItem(context.Background(), f.id, itemV, &f.itemA); err != nil {
		t.Fatal(err)
	}

	got := f.playback(t, f.itemA, f.a)
	if stepOf(got.Next) != [2]int64{f.itemG, f.members[1]} || stepOf(got.NextItem) != [2]int64{itemV, virtual.ID} {
		t.Errorf("next = %v, nextItem = %v, want G のメンバー 2 とバーチャルな項目", stepOf(got.Next), stepOf(got.NextItem))
	}
	if got.Count != 4 || got.PlayableCount != 3 {
		t.Errorf("count・playableCount = %d・%d, want 4・3", got.Count, got.PlayableCount)
	}
}

func TestPlaylistPlaybackNotFound(t *testing.T) {
	f := newPlaybackFixture(t)
	ctx := context.Background()
	if err := f.db.Playlists().RemovePlaylistItem(ctx, f.id, f.itemA); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Playlists().PlaylistPlayback(ctx, f.id, f.itemA, f.a); !errors.Is(err, domain.ErrPlaylistItemNotFound) {
		t.Errorf("外した項目: err = %v", err)
	}
	other := createPlaylist(t, f.db, "別")
	if _, err := f.db.Playlists().PlaylistPlayback(ctx, other, f.itemB, f.b); !errors.Is(err, domain.ErrPlaylistItemNotFound) {
		t.Errorf("別のプレイリストの項目: err = %v", err)
	}
	if err := f.db.Playlists().DeletePlaylist(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Playlists().PlaylistPlayback(ctx, f.id, f.itemB, f.b); !errors.Is(err, domain.ErrPlaylistNotFound) {
		t.Errorf("消したプレイリスト: err = %v", err)
	}
}

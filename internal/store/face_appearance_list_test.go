package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func insertFaceAppearance(t *testing.T, db *DB, key string, personID int64, dropped bool, tracks string, crop []byte) int64 {
	t.Helper()
	res, err := db.sql.Exec(`insert into face_appearances (content_key, recognizer, person_id, dropped, tracks,
		good_seconds, track_count, crop) values (?, 'sface_int8', ?, ?, ?, 10, 2, ?)`,
		key, personID, dropped, tracks, crop)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPersonAppearancesPageNewestVideoFirstWithoutDroppedOnes(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	paths := []string{fixturePath("/media/old.mp4"), fixturePath("/media/new.mp4")}
	ids := upsertAll(t, db, listingFile(paths[0], "Old", "key-old", 1), listingFile(paths[1], "New", "key-new", 2))
	person := createPerson(t, db, "Hanako").ID
	other := createPerson(t, db, "Taro").ID
	const track = `[[0, 10, 0.5, 0.5, 0.1]]`

	old := insertFaceAppearance(t, db, "key-old", person, false, track, []byte("crop"))
	// Overlapping tracks out of order are listed as joined spans in order.
	newest := insertFaceAppearance(t, db, "key-new", person, false,
		`[[20, 30, 0.5, 0.5, 0.1], [0, 10, 0.5, 0.5, 0.1], [5, 12, 0.5, 0.5, 0.1]]`, []byte("crop"))
	insertFaceAppearance(t, db, "key-new", person, true, track, []byte("crop"))
	insertFaceAppearance(t, db, "key-new", other, false, track, []byte("crop"))
	gone := insertFaceAppearance(t, db, "key-gone", person, false, track, nil)

	var got []domain.PersonAppearance
	cursor, pages := "", 0
	for {
		page, err := db.Credits().PersonAppearances(ctx, person, domain.PersonAppearanceQuery{Cursor: cursor, Limit: 1})
		if err != nil {
			t.Fatalf("page %d: %v", pages, err)
		}
		for _, item := range page.Items {
			item.AddedAt = 0
			got = append(got, item)
		}
		pages++
		// The dropped appearance and another person's are not counted, on every page.
		if page.Total != 3 {
			t.Errorf("page %d: total = %d, want 3", pages, page.Total)
		}
		if cursor = page.NextCursor; cursor == "" {
			break
		}
	}
	// The focus is the middle of the longest track, the first of equal ones.
	focus := func(time float64) *domain.FaceFocus { return &domain.FaceFocus{Time: time, X: 0.5, Y: 0.5, W: 0.1} }
	want := []domain.PersonAppearance{
		{ID: newest, VideoID: ids[paths[1]], Title: "New", Spans: []domain.FaceTimeSpan{{Start: 0, End: 12}, {Start: 20, End: 30}}, HasCrop: true,
			Focus: focus(25)},
		{ID: old, VideoID: ids[paths[0]], Title: "Old", Spans: []domain.FaceTimeSpan{{Start: 0, End: 10}}, HasCrop: true, Focus: focus(5)},
		{ID: gone, Title: "key-gone", Spans: []domain.FaceTimeSpan{{Start: 0, End: 10}}, Focus: focus(5)},
	}
	if !reflect.DeepEqual(got, want) || pages != 3 {
		t.Errorf("appearances over %d pages = %+v, want %+v over 3", pages, got, want)
	}

	all, err := db.Credits().PersonAppearances(ctx, person, domain.PersonAppearanceQuery{})
	for i := range all.Items {
		all.Items[i].AddedAt = 0
	}
	if err != nil || !reflect.DeepEqual(all.Items, want) || all.NextCursor != "" {
		t.Errorf("without a limit = %+v, %v; want every appearance and no cursor", all, err)
	}
	if _, err := db.Credits().PersonAppearances(ctx, person+100, domain.PersonAppearanceQuery{Limit: 1}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing person: err = %v, want ErrNotFound", err)
	}
	if _, err := db.Credits().PersonAppearances(ctx, person, domain.PersonAppearanceQuery{Cursor: "bad", Limit: 1}); !errors.Is(err, domain.ErrInvalidCursor) {
		t.Errorf("unreadable cursor: err = %v, want ErrInvalidCursor", err)
	}
}

func setAppearanceCentroid(t *testing.T, db *DB, id int64, v []float32) {
	t.Helper()
	if _, err := db.sql.Exec(`update face_appearances set centroid = ? where id = ?`,
		encodeFaceVector(domain.NormalizeFaceVector(v)), id); err != nil {
		t.Fatal(err)
	}
}

// The face least like the person's others comes first, or last in the reverse order, a face that cannot be compared
// last in both, and a cursor from one order is refused by the other.
func TestPersonAppearancesLeastSimilarFirst(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	person := createPerson(t, db, "Hanako").ID
	const track = `[[0, 10, 0.5, 0.5, 0.1]]`
	near := insertFaceAppearance(t, db, "key-a", person, false, track, nil)
	nearer := insertFaceAppearance(t, db, "key-b", person, false, track, nil)
	stranger := insertFaceAppearance(t, db, "key-c", person, false, track, nil)
	unscored := insertFaceAppearance(t, db, "key-d", person, false, track, nil)
	setAppearanceCentroid(t, db, near, []float32{1, 0.2, 0})
	setAppearanceCentroid(t, db, nearer, []float32{1, 0.05, 0})
	setAppearanceCentroid(t, db, stranger, []float32{0, 0, 1})

	var got []int64
	cursor := ""
	for {
		page, err := db.Credits().PersonAppearances(ctx, person,
			domain.PersonAppearanceQuery{Sort: domain.PersonAppearancesLeastSimilar, Cursor: cursor, Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			got = append(got, item.ID)
			if (item.Similarity == nil) != (item.ID == unscored) {
				t.Errorf("appearance %d similarity = %v", item.ID, item.Similarity)
			}
		}
		if cursor = page.NextCursor; cursor == "" {
			break
		}
		if _, err := db.Credits().PersonAppearances(ctx, person, domain.PersonAppearanceQuery{Cursor: cursor, Limit: 1}); !errors.Is(err, domain.ErrInvalidCursor) {
			t.Errorf("a leastSimilar cursor in the newest order: err = %v, want ErrInvalidCursor", err)
		}
	}
	if want := []int64{stranger, near, nearer, unscored}; !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	most, err := db.Credits().PersonAppearances(ctx, person, domain.PersonAppearanceQuery{Sort: domain.PersonAppearancesMostSimilar})
	if err != nil {
		t.Fatal(err)
	}
	got = got[:0]
	for _, item := range most.Items {
		got = append(got, item.ID)
	}
	if want := []int64{nearer, near, stranger, unscored}; !reflect.DeepEqual(got, want) {
		t.Errorf("mostSimilar order = %v, want %v", got, want)
	}
}

func TestFaceAppearanceCropIsGoneWithItsPerson(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	person := createPerson(t, db, "Hanako").ID
	id := insertFaceAppearance(t, db, "key-a", person, false, `[[0, 10, 0.5, 0.5, 0.1]]`, []byte("jpeg"))

	if crop, err := db.Credits().FaceAppearanceCrop(ctx, id); err != nil || string(crop) != "jpeg" {
		t.Fatalf("crop = %q, %v; want the stored JPEG", crop, err)
	}
	if err := db.Credits().Delete(ctx, domain.CreditPerson, person); err != nil {
		t.Fatal(err)
	}
	for _, target := range []int64{id, id + 100} {
		if _, err := db.Credits().FaceAppearanceCrop(ctx, target); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("crop of appearance %d: err = %v, want ErrNotFound", target, err)
		}
	}
}

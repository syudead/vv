package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeBackgroundCounts struct {
	remaining map[domain.BackgroundGroup]int
	keys      map[string]int64
}

func (f fakeBackgroundCounts) BackgroundRemaining(context.Context, domain.Audience) (map[domain.BackgroundGroup]int, error) {
	return f.remaining, nil
}

func (f fakeBackgroundCounts) VideoIDsByContentKey(_ context.Context, keys []string) (map[string]int64, error) {
	ids := map[string]int64{}
	for _, key := range keys {
		if id, ok := f.keys[key]; ok {
			ids[key] = id
		}
	}
	return ids, nil
}

type fakeBackgroundScans struct {
	scan *domain.Scan
	held []domain.DirtyDirectory
}

func (f fakeBackgroundScans) CurrentScan(context.Context) (domain.Scan, error) {
	if f.scan == nil {
		return domain.Scan{}, domain.ErrNotFound
	}
	return *f.scan, nil
}

func (f fakeBackgroundScans) HeldWatchDirectories(context.Context) ([]domain.DirtyDirectory, error) {
	return f.held, nil
}

// fakeBackgroundVideos answers the videos the owner can see; FolderOff marks one only under a switched-off folder.
type fakeBackgroundVideos map[int64]domain.Video

func (f fakeBackgroundVideos) GetVideo(_ context.Context, _ domain.Audience, id int64) (domain.Video, error) {
	video, ok := f[id]
	if !ok {
		return domain.Video{}, domain.ErrNotFound
	}
	return video, nil
}

type backgroundStatusRig struct {
	scans  *Scans
	counts fakeBackgroundCounts
	store  fakeBackgroundScans
	videos fakeBackgroundVideos
	off    map[domain.JobKind]bool
}

func newBackgroundStatusRig() *backgroundStatusRig {
	return &backgroundStatusRig{
		scans: NewScans(ScansOptions{Logger: discardLogger()}),
		counts: fakeBackgroundCounts{
			remaining: map[domain.BackgroundGroup]int{}, keys: map[string]int64{"key-shown": 1, "key-off": 2},
		},
		videos: fakeBackgroundVideos{1: {ID: 1, Title: "Shown"}, 2: {ID: 2, Title: "Off", FolderOff: true}},
	}
}

func (r *backgroundStatusRig) read(t *testing.T) domain.BackgroundWork {
	t.Helper()
	status := NewBackgroundStatus(BackgroundStatusOptions{
		Counts: r.counts, Scans: r.store, Videos: r.videos, Running: r.scans,
		FeatureOff: func(context.Context) (map[domain.JobKind]bool, error) { return r.off, nil },
	})
	var work domain.BackgroundWork
	if err := status.Read(context.Background(), domain.AudienceOwner, &work); err != nil {
		t.Fatal(err)
	}
	return work
}

// A running auto_tag job is listed with the video its content key names until its handler returns; a job whose video
// lies only under a switched-off folder is listed without it.
func TestBackgroundStatusListsRunningJobsWithTheirVideos(t *testing.T) {
	rig := newBackgroundStatusRig()
	shown := domain.Job{ID: 10, Kind: domain.JobAutoTag, ContentKey: "key-shown"}
	off := domain.Job{ID: 11, Kind: domain.JobProbe, VideoID: 2, LocationPath: fixturePath("/media/off/clip.mp4")}
	rig.scans.JobStarted(shown)
	rig.scans.JobStarted(off)

	want := []domain.BackgroundActivity{
		{Group: domain.BackgroundAutoTag, Kind: "auto_tag", Video: &domain.BackgroundVideo{ID: 1, Title: "Shown"}},
		{Group: domain.BackgroundImport, Kind: "probe", FileName: "clip.mp4"},
	}
	if got := rig.read(t).Running; !reflect.DeepEqual(got, want) {
		t.Fatalf("running = %+v, want %+v", got, want)
	}

	rig.scans.JobFinished(shown)
	if got := rig.read(t).Running; !reflect.DeepEqual(got, want[1:]) {
		t.Fatalf("running after the auto_tag job = %+v, want %+v", got, want[1:])
	}
}

// A group whose feature holds its kinds back says so, so a count that does not fall after a resume is explained.
func TestBackgroundStatusMarksAGroupWhoseFeatureIsOff(t *testing.T) {
	rig := newBackgroundStatusRig()
	rig.counts.remaining[domain.BackgroundAutoTag] = 3
	rig.off = map[domain.JobKind]bool{domain.JobAutoTag: true}

	want := domain.EmptyBackgroundGroups()
	want[2] = domain.BackgroundGroupStatus{Group: domain.BackgroundAutoTag, Remaining: 3, FeatureOff: true}
	if got := rig.read(t).Groups; !reflect.DeepEqual(got, want) {
		t.Fatalf("groups = %+v, want %+v", got, want)
	}
}

// The scan group counts the files a paused scan has not read and waits for the resume; held watch directories wait
// too, with no scan open.
func TestBackgroundStatusScanWaitsForAPausedScanOrHeldDirectories(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store fakeBackgroundScans
		want  domain.BackgroundGroupStatus
	}{
		{
			name:  "paused scan",
			store: fakeBackgroundScans{scan: &domain.Scan{State: domain.ScanPaused, Total: 10, Completed: 4, Failed: 1}},
			want:  domain.BackgroundGroupStatus{Group: domain.BackgroundScan, Remaining: 5, Waiting: true},
		},
		{
			name: "held directories",
			store: fakeBackgroundScans{
				scan: &domain.Scan{State: domain.ScanDone, Total: 10, Completed: 10},
				held: []domain.DirtyDirectory{{Path: fixturePath("/media/a")}},
			},
			want: domain.BackgroundGroupStatus{Group: domain.BackgroundScan, Waiting: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newBackgroundStatusRig()
			rig.store = tc.store
			if got := rig.read(t).Groups[0]; got != tc.want {
				t.Fatalf("scan group = %+v, want %+v", got, tc.want)
			}
		})
	}
}

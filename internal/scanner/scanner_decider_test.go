package scanner

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 走査は始めたもの（decider）を、積む仕事ごとに待ち行列へ渡す。監視の走査は規則が始めたものとして積む
func TestScanQueuesJobsWithItsDecider(t *testing.T) {
	for _, tc := range []struct {
		name string
		scan func(*Scanner, string) error
		want domain.ActivityDecider
	}{
		{"manual", func(s *Scanner, _ string) error {
			_, err := s.Scan(context.Background(), domain.ActivityByOwner)
			return err
		}, domain.ActivityByOwner},
		{"resume", func(s *Scanner, _ string) error {
			_, err := s.Scan(context.Background(), domain.ActivityByRule)
			return err
		}, domain.ActivityByRule},
		{"watch", func(s *Scanner, root string) error {
			_, err := s.ScanScoped(context.Background(), []domain.DirtyDirectory{{Path: root, Recursive: true}})
			return err
		}, domain.ActivityByRule},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := mediaTree(t, map[string]string{"a.mp4": "a", "b.mkv": "b"})
			index := newFakeIndex()
			if err := tc.scan(scopedScanner(root, index), root); err != nil {
				t.Fatal(err)
			}
			if len(index.deciders) == 0 {
				t.Fatal("no job was queued")
			}
			if slices.ContainsFunc(index.deciders, func(d domain.ActivityDecider) bool { return d != tc.want }) {
				t.Errorf("deciders = %v, want every job queued by %s", index.deciders, tc.want)
			}
		})
	}
}

// 新しい動画に要る仕事は 1 回の呼び出しでまとめて積む。別々に確定すると、その間に前の段階が終わって記録が
// 閉じ、残りの段階が 2 件目の import の記録を開く。
func TestScanQueuesANewVideosJobsInOneCall(t *testing.T) {
	root := mediaTree(t, map[string]string{"a.mp4": "a"})
	index := newFakeIndex()
	if _, err := scopedScanner(root, index).Scan(context.Background(), domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}
	if len(index.jobs) < 2 {
		t.Fatalf("jobs = %v, want the new video's stages", index.jobs)
	}
	if index.enqueueCalls != 1 {
		t.Errorf("EnqueueJobs calls = %d, want 1 for %d jobs", index.enqueueCalls, len(index.jobs))
	}
}

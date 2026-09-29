package domain

import "testing"

func TestVideoRefValid(t *testing.T) {
	cases := []struct {
		ref  VideoRef
		want bool
	}{
		{VideoRef{ID: 1}, true},
		{VideoRef{ContentKey: "k"}, true},
		{VideoRef{Path: "/media/a.mp4"}, true},
		{VideoRef{}, false},
		{VideoRef{ID: -1}, false},
		{VideoRef{ID: 1, Path: "/media/a.mp4"}, false},
		{VideoRef{ContentKey: "k", Path: "/media/a.mp4"}, false},
		{VideoRef{ID: 1, ContentKey: "k", Path: "/media/a.mp4"}, false},
	}
	for _, tc := range cases {
		if got := tc.ref.Valid(); got != tc.want {
			t.Errorf("%+v: Valid() = %v, want %v", tc.ref, got, tc.want)
		}
	}
}

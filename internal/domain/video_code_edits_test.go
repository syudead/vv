package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeVideoCodeEdits(t *testing.T) {
	items := func(n int) []VideoCodeEdit {
		out := make([]VideoCodeEdit, n)
		for i := range out {
			out[i] = VideoCodeEdit{VideoID: int64(i + 1), ContentVersion: "v", Code: "ABC-1"}
		}
		return out
	}
	withCode := func(code string) []VideoCodeEdit {
		return []VideoCodeEdit{{VideoID: 1, Code: "ok"}, {VideoID: 2, Code: code}}
	}
	for _, tc := range []struct {
		name    string
		items   []VideoCodeEdit
		problem WorkInfoProblem
		wantErr error
	}{
		{name: "no items", items: nil, wantErr: ErrVideoCodeEditCount},
		{name: "1,001 items", items: items(1001), wantErr: ErrVideoCodeEditCount},
		{name: "a repeated video", items: []VideoCodeEdit{{VideoID: 1, Code: "a"}, {VideoID: 1, Code: "b"}},
			wantErr: ErrDuplicateVideoCodeEdit},
		{name: "101 code points", items: withCode(strings.Repeat("あ", 101)), problem: WorkInfoTooLong},
		{name: "a control character", items: withCode("ABC\t1"), problem: WorkInfoControlCharacters},
		{name: "blank", items: withCode("  "), problem: WorkInfoEmpty},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeVideoCodeEdits(tc.items)
			if errors.Is(tc.wantErr, ErrVideoCodeEditCount) {
				if !errors.Is(err, ErrVideoCodeEditCount) {
					t.Fatalf("err = %v, want ErrVideoCodeEditCount", err)
				}
				return
			}
			var at *VideoCodeEditAtError
			if !errors.As(err, &at) || at.Index != 1 {
				t.Fatalf("err = %v, want an error at item 1", err)
			}
			var invalid *InvalidWorkInfoError
			switch {
			case tc.wantErr != nil && !errors.Is(err, tc.wantErr):
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			case tc.problem != 0 && (!errors.As(err, &invalid) || invalid.Problem != tc.problem || invalid.Field != "code"):
				t.Errorf("err = %v, want problem %d at code", err, tc.problem)
			}
		})
	}
}

func TestNormalizeVideoCodeEditsAcceptsTheLimits(t *testing.T) {
	full := make([]VideoCodeEdit, VideoCodeEditsMaxItems)
	for i := range full {
		full[i] = VideoCodeEdit{VideoID: int64(i + 1), Code: "x"}
	}
	full[0].Code = " " + strings.Repeat("あ", 100) + " "
	got, err := NormalizeVideoCodeEdits(full)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != VideoCodeEditsMaxItems || got[0].Code != strings.Repeat("あ", 100) {
		t.Errorf("got %d items, first code %q; want 1,000 items and the trimmed 100-code-point code", len(got), got[0].Code)
	}
}

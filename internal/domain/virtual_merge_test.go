package domain

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// The combined description counts the blank line between the two, and the combined links count after removing
// links both sides have; one past either limit is rejected.
func TestMergeWorkInfoLimits(t *testing.T) {
	links := func(from, count int) []string {
		out := make([]string, 0, count)
		for i := from; i < from+count; i++ {
			out = append(out, "https://example.com/"+strconv.Itoa(i))
		}
		return out
	}
	for _, tc := range []struct {
		name          string
		file, virtual WorkInfo
		want          error
	}{
		{"説明がちょうど上限", WorkInfo{Description: strings.Repeat("あ", 5_000)}, WorkInfo{Description: strings.Repeat("い", 4_998)}, nil},
		{"説明が上限を 1 超える", WorkInfo{Description: strings.Repeat("あ", 5_000)}, WorkInfo{Description: strings.Repeat("い", 4_999)}, ErrMergedDescriptionTooLong},
		{"重なるリンクを除いてちょうど 20", WorkInfo{URLs: links(0, 15)}, WorkInfo{URLs: links(10, 10)}, nil},
		{"リンクが 21", WorkInfo{URLs: links(0, 15)}, WorkInfo{URLs: links(10, 11)}, ErrMergedTooManyURLs},
	} {
		if _, err := MergeWorkInfo(tc.file, tc.virtual); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
}

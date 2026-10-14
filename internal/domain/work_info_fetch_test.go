package domain

import (
	"errors"
	"strings"
	"testing"
)

// 取得の URL の上限は契約どおり文字数で数え、多バイトの文字を含む URL を UTF-8 のバイト数で拒まない
// （specs/049-work-info-plugins/contracts/screen-api.md）。
func TestCheckWorkInfoFetchURLCountsCharacters(t *testing.T) {
	prefix := "https://example.com/"
	atLimit := prefix + strings.Repeat("作", WorkInfoFetchURLMaxRunes-len(prefix))
	if len(atLimit) <= WorkInfoFetchURLMaxRunes {
		t.Fatalf("the URL must be longer than the limit in bytes: %d", len(atLimit))
	}
	if err := CheckWorkInfoFetchURL(atLimit); err != nil {
		t.Fatalf("CheckWorkInfoFetchURL(%d characters) = %v, want nil", WorkInfoFetchURLMaxRunes, err)
	}

	cases := map[string]string{
		"over the limit": atLimit + "品",
		"invalid UTF-8":  prefix + "\xff",
		"not http":       "ftp://example.com/",
		"no host":        "https:///path",
		"empty":          "",
	}
	for name, raw := range cases {
		if err := CheckWorkInfoFetchURL(raw); !errors.Is(err, ErrWorkInfoRunInvalid) {
			t.Errorf("%s: CheckWorkInfoFetchURL = %v, want ErrWorkInfoRunInvalid", name, err)
		}
	}
}

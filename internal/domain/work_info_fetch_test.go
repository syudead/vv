package domain

import (
	"errors"
	"reflect"
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

// 実行の要求は設定ごとの実効値を運ぶ。未設定のキーと選択肢の外の保存値には既定値、設定済みのキーには保存値で、
// 既定値のある必須のキーは不足にならない（specs/056-plugin-tooling/research.md R-3）。
func TestWorkInfoPluginEffectiveValues(t *testing.T) {
	settings := []WorkInfoPluginSetting{
		{Key: "unset", Label: "L", Required: true, Default: "d1"},
		{Key: "stored", Label: "L", Default: "d2"},
		{Key: "stale", Label: "L", Default: "en", Options: []string{"ja", "en"}},
		{Key: "chosen", Label: "L", Default: "en", Options: []string{"ja", "en"}},
		{Key: "free", Label: "L"},
		{Key: "needed", Label: "L", Required: true},
		{Key: "staleNoDefault", Label: "L", Options: []string{"a", "b"}},
	}
	values := map[string]string{"stored": "mine", "stale": "fr", "chosen": "ja", "staleNoDefault": "c", "other": "x"}

	got := DeclaredWorkInfoPluginValues(settings, values)
	want := map[string]string{"unset": "d1", "stored": "mine", "stale": "en", "chosen": "ja"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("request settings = %v, want %v", got, want)
	}
	if missing := MissingWorkInfoPluginSettings(settings, values); !reflect.DeepEqual(missing, []string{"needed"}) {
		t.Fatalf("missing = %v, want [needed]", missing)
	}
	for _, tc := range []struct {
		key   string
		value string
		stale bool
	}{{"stale", "en", true}, {"chosen", "ja", false}, {"staleNoDefault", "", true}, {"unset", "d1", false}} {
		var s WorkInfoPluginSetting
		for _, d := range settings {
			if d.Key == tc.key {
				s = d
			}
		}
		if value, stale := EffectiveWorkInfoPluginValue(s, values); value != tc.value || stale != tc.stale {
			t.Fatalf("%s = %q, stale %v; want %q, %v", tc.key, value, stale, tc.value, tc.stale)
		}
	}
}

func TestWorkInfoPluginEntryReportsAStaleValue(t *testing.T) {
	plugin := WorkInfoPlugin{
		Manifest: WorkInfoPluginManifest{Settings: []WorkInfoPluginSetting{
			{Key: "lang", Label: "L", Default: "en", Options: []string{"ja", "en"}},
			{Key: "other", Label: "L", Options: []string{"ja", "en"}},
		}},
		Values: map[string]string{"lang": "fr", "other": "ja"},
	}
	settings := plugin.Entry().Settings
	if s := settings[0]; !s.Set || s.Value != "fr" || s.StaleValue != "fr" || s.Default != "en" {
		t.Fatalf("stale entry = %+v", s)
	}
	if s := settings[1]; !s.Set || s.Value != "ja" || s.StaleValue != "" {
		t.Fatalf("entry inside the options = %+v", s)
	}
}

package domain

import (
	"reflect"
	"testing"
)

func TestMaskWorkInfoRunLog(t *testing.T) {
	cases := []struct {
		name      string
		stderr    string
		truncated bool
		secrets   []string
		want      string
	}{
		{"2 回出る秘密の値は 2 回伏せる", "key=tok123 retry key=tok123\n", false, []string{"tok123"},
			"key=[secret] retry key=[secret]\n"},
		{"秘密でない値は残す", "lang=ja key=tok123", false, []string{"tok123"}, "lang=ja key=[secret]"},
		{"空の秘密はどこにも一致しない", "nothing secret here", false, []string{""}, "nothing secret here"},
		{"ほかの値を含む値は丸ごと伏せる", "abc-long abc", false, []string{"abc", "abc-long"}, "[secret] [secret]"},
		{"切り詰めた出力の末尾の秘密の先頭を伏せる", "error: key=tok", true, []string{"tok123"}, "error: key=[secret]"},
		{"先頭の 1 バイトでも伏せる", "error: key=t", true, []string{"tok123"}, "error: key=[secret]"},
		{"切り詰めていなければ末尾は伏せない", "error: key=tok", false, []string{"tok123"}, "error: key=tok"},
		{"切り詰めた出力でも、末尾が秘密の先頭でなければそのまま", "error: done", true, []string{"tok123"}, "error: done"},
		{"末尾の丸ごとの値と、その後ろの先頭を両方伏せる", "tok123tok", true, []string{"tok123"}, "[secret][secret]"},
		{"重なる 2 つの値はまとめて伏せる", "x abcde y", false, []string{"abc", "cde"}, "x [secret] y"},
		{"自己重複する値の一致をまとめて伏せる", "k=aaa", false, []string{"aa"}, "k=[secret]"},
		{"末尾の先頭が丸ごとの値と重なってもまとめて伏せる", "k=aba", true, []string{"aba"}, "k=[secret]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			log := WorkInfoRunLog{Operation: WorkInfoFetch, Failure: WorkInfoRunFailed, Stderr: c.stderr, Truncated: c.truncated}
			got := MaskWorkInfoRunLog(log, c.secrets)
			want := log
			want.Stderr = c.want
			if got != want {
				t.Fatalf("MaskWorkInfoRunLog = %+v, want %+v", got, want)
			}
		})
	}
}

func TestSecretWorkInfoPluginValues(t *testing.T) {
	settings := []WorkInfoPluginSetting{
		{Key: "apiKey", Secret: true}, {Key: "lang"}, {Key: "token", Secret: true}, {Key: "unset", Secret: true},
	}
	got := SecretWorkInfoPluginValues(settings, map[string]string{"apiKey": "a", "lang": "ja", "token": "t"})
	if want := []string{"a", "t"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SecretWorkInfoPluginValues = %v, want %v", got, want)
	}
}

package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseWorkInfoPluginProxyURLAcceptsTheContractSchemes(t *testing.T) {
	for _, raw := range []string{
		"http://proxy.example:8080",
		"http://user:secret@proxy.example:8080",
		"https://proxy.example",
		"https://user@proxy.example:443",
		"socks5://192.168.1.2:1080",
		"socks5://user:secret@[fd00::2]:1080",
	} {
		u, err := ParseWorkInfoPluginProxyURL(raw)
		if err != nil || u == nil || u.String() != raw {
			t.Errorf("ParseWorkInfoPluginProxyURL(%q) = %v, %v, want the URL", raw, u, err)
		}
	}
}

func TestParseWorkInfoPluginProxyURLEmptyMeansNoProxy(t *testing.T) {
	u, err := ParseWorkInfoPluginProxyURL("")
	if u != nil || err != nil {
		t.Fatalf(`ParseWorkInfoPluginProxyURL("") = %v, %v, want no proxy`, u, err)
	}
}

func TestParseWorkInfoPluginProxyURLCountsCharacters(t *testing.T) {
	// Exactly 2,048 code points but more than 2,048 UTF-8 bytes.
	host := strings.Repeat("ü", WorkInfoPluginProxyURLMaxLen-len("http://.example")) + ".example"
	if u, err := ParseWorkInfoPluginProxyURL("http://" + host); err != nil || u == nil {
		t.Fatalf("ParseWorkInfoPluginProxyURL(2048 characters) = %v, %v, want a proxy", u, err)
	}
	if _, err := ParseWorkInfoPluginProxyURL("http://ü" + host); !errors.Is(err, ErrWorkInfoPluginProxyURLInvalid) {
		t.Fatalf("ParseWorkInfoPluginProxyURL(2049 characters) = %v, want ErrWorkInfoPluginProxyURLInvalid", err)
	}
}

func TestParseWorkInfoPluginProxyURLRefusesOtherURLs(t *testing.T) {
	long := "http://" + strings.Repeat("a", WorkInfoPluginProxyURLMaxLen) + ".example"
	for _, raw := range []string{
		"http://proxy.example:8080/path",
		"http://proxy.example/",
		"http://proxy.example?q=1",
		"http://proxy.example#top",
		"http://proxy.example#",
		"http://proxy.example:",
		"http://[::1]:",
		"ftp://proxy.example",
		"socks4://proxy.example:1080",
		"http://",
		"http://:8080",
		"proxy.example:8080",
		"http://proxy.example:0",
		"http://proxy.example:70000",
		"http://proxy.example:port",
		long,
	} {
		if u, err := ParseWorkInfoPluginProxyURL(raw); !errors.Is(err, ErrWorkInfoPluginProxyURLInvalid) {
			t.Errorf("ParseWorkInfoPluginProxyURL(%.40q) = %v, %v, want ErrWorkInfoPluginProxyURLInvalid", raw, u, err)
		}
	}
}

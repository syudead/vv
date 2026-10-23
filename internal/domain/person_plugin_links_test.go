package domain

import (
	"errors"
	"slices"
	"testing"
)

func TestLinkMatchesDomainsByHostSuffixOverHTTPOnly(t *testing.T) {
	domains := []string{"example.com"}
	for _, tc := range []struct {
		link string
		want bool
	}{
		{"https://example.com/p/1", true},
		{"http://WWW.Example.COM:8080/p/1", true},
		{"https://m.example.com/p/1", true},
		{"https://badexample.com/p/1", false},
		{"https://example.com.evil.test/p/1", false},
		{"ftp://example.com/p/1", false},
		{"example.com/p/1", false},
	} {
		if got := LinkMatchesDomains(tc.link, domains); got != tc.want {
			t.Errorf("LinkMatchesDomains(%q) = %v; want %v", tc.link, got, tc.want)
		}
	}
}

func TestPersonFetchLinksFoldsSchemeHostCaseAndTrailingSlashOnly(t *testing.T) {
	links := []string{
		"https://other.test/a",
		"http://Example.com/a/",
		"https://example.com/a",
		"https://www.example.com/a?id=1",
		"https://WWW.example.com/a/?id=1",
		"https://www.example.com/a?id=2",
		"https://example.com/#/p/1",
		"https://example.com/#/p/2",
		"https://example.com/A",
	}
	got := PersonFetchLinks(links, []string{"example.com"})
	want := []string{
		"http://Example.com/a/",
		"https://www.example.com/a?id=1",
		"https://www.example.com/a?id=2",
		"https://example.com/#/p/1",
		"https://example.com/#/p/2",
		"https://example.com/A",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("PersonFetchLinks = %q; want %q", got, want)
	}
}

func TestNormalizePersonFetchLinkAcceptsOnlyHTTPURLs(t *testing.T) {
	if got, err := NormalizePersonFetchLink("  https://example.com/p/1  "); err != nil || got != "https://example.com/p/1" {
		t.Fatalf("NormalizePersonFetchLink = %q, %v; want the trimmed link", got, err)
	}
	for _, raw := range []string{"", "Hanako", "ftp://example.com/p/1", "https:///p/1", "https://example.com/\n"} {
		if _, err := NormalizePersonFetchLink(raw); !errors.Is(err, ErrWorkInfoRunInvalid) {
			t.Errorf("NormalizePersonFetchLink(%q) err = %v; want ErrWorkInfoRunInvalid", raw, err)
		}
	}
}

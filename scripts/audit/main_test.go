package main

import (
	"reflect"
	"testing"
)

// A direct advisory on picomatch, inherited by tinyglobby and vite.
const report = `{
  "vulnerabilities": {
    "picomatch": {"via": [{"name": "picomatch", "severity": "high", "range": "<=4.0.7", "title": "ReDoS", "url": "https://github.com/advisories/GHSA-aaaa-bbbb-cccc"}]},
    "tinyglobby": {"via": ["picomatch"]},
    "vite": {"via": ["picomatch", "tinyglobby", {"name": "vite", "severity": "moderate", "range": "<8.0.1", "title": "Path traversal", "url": "https://github.com/advisories/GHSA-dddd-eeee-ffff"}]}
  }
}`

func TestParseAdvisoriesListsEachAdvisoryOnce(t *testing.T) {
	found, err := parseAdvisories([]byte(report))
	if err != nil {
		t.Fatal(err)
	}
	want := []advisory{
		{ID: "GHSA-aaaa-bbbb-cccc", Package: "picomatch", Severity: "high", Range: "<=4.0.7", Title: "ReDoS", URL: "https://github.com/advisories/GHSA-aaaa-bbbb-cccc"},
		{ID: "GHSA-dddd-eeee-ffff", Package: "vite", Severity: "moderate", Range: "<8.0.1", Title: "Path traversal", URL: "https://github.com/advisories/GHSA-dddd-eeee-ffff"},
	}
	if !reflect.DeepEqual(found, want) {
		t.Fatalf("got %+v, want %+v", found, want)
	}
}

func TestParseAdvisoriesRejectsAnNpmError(t *testing.T) {
	if _, err := parseAdvisories([]byte(`{"error": {"summary": "registry unreachable"}}`)); err == nil {
		t.Fatal("an npm error report must not read as a clean audit")
	}
}

func TestUnacceptedKeepsAdvisoriesWithoutAMatchingAcceptance(t *testing.T) {
	found := []advisory{
		{ID: "GHSA-aaaa-bbbb-cccc", Package: "picomatch"},
		{ID: "GHSA-dddd-eeee-ffff", Package: "vite"},
		{ID: "GHSA-gggg-hhhh-iiii", Package: "minimatch"},
	}
	accepted := []acceptance{
		{ID: "GHSA-aaaa-bbbb-cccc", Package: "picomatch"},
		// The same ID on another package is a different finding.
		{ID: "GHSA-dddd-eeee-ffff", Package: "rollup"},
	}
	got := unaccepted(found, accepted)
	want := []advisory{found[1], found[2]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestStaleListsAcceptancesNoProjectHas(t *testing.T) {
	accepted := []acceptance{
		{ID: "GHSA-aaaa-bbbb-cccc", Package: "picomatch"},
		{ID: "GHSA-dddd-eeee-ffff", Package: "vite"},
	}
	got := stale(accepted, map[string]bool{"GHSA-aaaa-bbbb-cccc": true})
	if !reflect.DeepEqual(got, accepted[1:]) {
		t.Fatalf("got %+v, want %+v", got, accepted[1:])
	}
}

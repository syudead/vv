package domain

import (
	"runtime"
	"testing"
)

func TestPathWithinRootUsesWindowsCaseRules(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path comparison")
	}
	if !PathWithinRoot(`C:\Media`, `c:\media`) {
		t.Fatal("same Windows path with different casing was not recognized")
	}
	if !PathWithinRoot(`C:\Media`, `c:\MEDIA\Movies\movie.mp4`) {
		t.Fatal("Windows descendant with different casing was not recognized")
	}
}

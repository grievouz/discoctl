package cmd

import "testing"

func TestBuildVersionForRelease(t *testing.T) {
	oldVersion, oldBuildDate := version, buildDate
	t.Cleanup(func() {
		version, buildDate = oldVersion, oldBuildDate
	})

	version = "v1.2.3"
	buildDate = "2026-08-11"

	want := "discoctl version 1.2.3 (2026-08-11)\n" + releaseURL + "1.2.3"
	if got := buildVersion(); got != want {
		t.Fatalf("buildVersion() = %q, want %q", got, want)
	}
}

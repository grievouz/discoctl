package auth

import (
	"strings"
	"testing"
)

func TestPublicTunnelURLFromText(t *testing.T) {
	t.Parallel()

	publicURL, ok := publicTunnelURLFromText(
		"example-123.lhr.life tunneled with tls termination, https://example-123.lhr.life",
	)
	if !ok || publicURL != "https://example-123.lhr.life" {
		t.Fatalf("URL = %q, detected = %t", publicURL, ok)
	}
}

func TestPublicTunnelURLFromTextRejectsOtherHosts(t *testing.T) {
	t.Parallel()

	for _, text := range []string{
		"https://localhost.run",
		"https://localhost.run.example.com",
		"http://example.lhr.life",
		"To set up custom domains go to https://admin.localhost.run/",
		"Documentation is at https://docs.localhost.run/",
	} {
		if publicURL, ok := publicTunnelURLFromText(text); ok {
			t.Fatalf("unexpected URL %q from %q", publicURL, text)
		}
	}
}

func TestPublicTunnelURLFromTextAcceptsLocalhostRunTunnelAnnouncement(t *testing.T) {
	t.Parallel()

	publicURL, ok := publicTunnelURLFromText(
		"example.localhost.run tunneled with tls termination, https://example.localhost.run",
	)
	if !ok || publicURL != "https://example.localhost.run" {
		t.Fatalf("URL = %q, detected = %t", publicURL, ok)
	}
}

func TestScanTunnelOutputKeepsDiagnosticTail(t *testing.T) {
	t.Parallel()

	urls := make(chan string, 1)
	tails := make(chan string, 1)
	scanTunnelOutput(strings.NewReader("first\nsecond\nthird\nfourth\nfifth\n"), urls, tails)
	if tail := <-tails; tail != "second; third; fourth; fifth" {
		t.Fatalf("tail = %q", tail)
	}
}

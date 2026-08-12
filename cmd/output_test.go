package cmd

import (
	"bytes"
	"flag"
	"io"
	"strings"
	"testing"
)

func TestWriteJSONIsCompactByDefault(t *testing.T) {
	var stdout bytes.Buffer
	output := &outputOptions{}
	if err := output.writeJSON(&stdout, struct {
		Value string `json:"value"`
	}{Value: "<ok>"}, nil, nil); err != nil {
		t.Fatal(err)
	}

	want := "{\"schema_version\":\"1\",\"data\":{\"value\":\"<ok>\"}}\n"
	if got := stdout.String(); got != want {
		t.Fatalf("output mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestWriteJSONPretty(t *testing.T) {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	output := addJSONFlag(flags)
	if err := flags.Parse([]string{"--json", "--pretty"}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := output.writeJSON(&stdout, []string{"one", "two"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !strings.Contains(got, "\n  \"schema_version\": \"1\",") {
		t.Fatalf("expected indented JSON, got %q", got)
	}
	if !strings.Contains(got, "\n    \"one\",") {
		t.Fatalf("expected indented array, got %q", got)
	}
}

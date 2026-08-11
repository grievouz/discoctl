package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestStatusSetDryRunDoesNotRequireAuthentication(t *testing.T) {
	t.Setenv("DISCOCTL_TOKEN", "")

	var stdout, stderr bytes.Buffer
	err := RunContext(
		context.Background(),
		[]string{
			"status", "set",
			"--presence", "dnd",
			"--text", "heads down",
			"--emoji", "🌸",
			"--dry-run",
		},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		SchemaVersion string     `json:"schema_version"`
		Data          statusPlan `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if result.SchemaVersion != schemaVersion {
		t.Fatalf("schema version = %q", result.SchemaVersion)
	}
	if !result.Data.DryRun || result.Data.Presence != "dnd" {
		t.Fatalf("unexpected status plan: %#v", result.Data)
	}
	if result.Data.Custom == nil || result.Data.Custom.Text != "heads down" || result.Data.Custom.EmojiName != "🌸" {
		t.Fatalf("custom status = %#v", result.Data.Custom)
	}
}

func TestStatusSetDryRunParsesCustomEmoji(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	err := RunContext(
		context.Background(),
		[]string{
			"status", "set",
			"--text", "party",
			"--emoji", "<a:party:423456789012345678>",
			"--dry-run",
		},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Data statusPlan `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Presence != "preserve_current" || result.Data.Custom == nil {
		t.Fatalf("unexpected status plan: %#v", result.Data)
	}
	if result.Data.Custom.EmojiID == nil || *result.Data.Custom.EmojiID != "423456789012345678" {
		t.Fatalf("custom emoji id = %#v", result.Data.Custom.EmojiID)
	}
}

func TestStatusClearDryRun(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	err := RunContext(
		context.Background(),
		[]string{"status", "clear", "--dry-run"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Data statusPlan `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Data.DryRun || result.Data.Action != "clear_custom_status" || result.Data.Custom != nil {
		t.Fatalf("unexpected clear plan: %#v", result.Data)
	}
}

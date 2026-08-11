package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestProfileUpdateDryRunDoesNotRequireAuthentication(t *testing.T) {
	t.Setenv("DISCOCTL_TOKEN", "")

	var stdout, stderr bytes.Buffer
	err := RunContext(
		context.Background(),
		[]string{
			"profile", "update",
			"--bio", "building tiny tools",
			"--color", "#ff00aa",
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
		SchemaVersion string            `json:"schema_version"`
		Data          profileUpdatePlan `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if result.SchemaVersion != schemaVersion {
		t.Fatalf("schema version = %q", result.SchemaVersion)
	}
	if !result.Data.DryRun || result.Data.Bio == nil || *result.Data.Bio != "building tiny tools" {
		t.Fatalf("unexpected profile plan: %#v", result.Data)
	}
	if result.Data.Color == nil || *result.Data.Color != "#FF00AA" {
		t.Fatalf("accent color = %#v", result.Data.Color)
	}
}

func TestProfileUpdateDryRunCanClearFields(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	err := RunContext(
		context.Background(),
		[]string{
			"profile", "update",
			"--clear-bio",
			"--clear-color",
			"--clear-avatar",
			"--clear-banner",
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
		Data profileUpdatePlan `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Avatar == nil || result.Data.Avatar.Operation != "clear" {
		t.Fatalf("avatar plan = %#v", result.Data.Avatar)
	}
	if result.Data.Banner == nil || result.Data.Banner.Operation != "clear" {
		t.Fatalf("banner plan = %#v", result.Data.Banner)
	}
}

func TestParseProfileColor(t *testing.T) {
	t.Parallel()

	value, normalized, err := parseProfileColor(" 12abEF ")
	if err != nil {
		t.Fatal(err)
	}
	if value != 0x12ABEF || normalized != "#12ABEF" {
		t.Fatalf("color = %d, %q", value, normalized)
	}

	if _, _, err := parseProfileColor("#abcd"); err == nil {
		t.Fatal("expected invalid color error")
	}
}

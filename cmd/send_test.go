package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageSendDryRunDoesNotRequireAuthentication(t *testing.T) {
	t.Setenv("DISCOCTL_TOKEN", "")

	var stdout, stderr bytes.Buffer
	err := RunContext(
		context.Background(),
		[]string{"messages", "send", "--channel", "223456789012345678", "--msg", "hello", "--dry-run"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		SchemaVersion string         `json:"schema_version"`
		Data          plannedMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if result.SchemaVersion != schemaVersion {
		t.Fatalf("schema version = %q", result.SchemaVersion)
	}
	if !result.Data.DryRun || result.Data.Content != "hello" {
		t.Fatalf("unexpected plan: %#v", result.Data)
	}
	if got := strings.Join(result.Data.AllowedMentions, ","); got != "users,roles,everyone" {
		t.Fatalf("allowed mentions = %q", got)
	}
	if result.Data.PingReplyAuthor {
		t.Fatal("a non-reply should not report a reply-author ping")
	}
	if result.Data.UnreadGuard != "enforced_at_send" {
		t.Fatalf("unread guard = %q", result.Data.UnreadGuard)
	}
}

func TestMessageSendDryRunCanBypassUnreadGuard(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	err := RunContext(
		context.Background(),
		[]string{
			"messages", "send", "--channel", "223456789012345678",
			"--msg", "hello", "--ignore-unread", "--dry-run",
		},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data plannedMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.UnreadGuard != "bypassed" {
		t.Fatalf("unread guard = %q", result.Data.UnreadGuard)
	}
}

func TestMessageReplyDryRunAcceptsURLBeforeFlags(t *testing.T) {
	t.Parallel()

	const messageURL = "https://discord.com/channels/@me/223456789012345678/323456789012345678"
	var stdout, stderr bytes.Buffer
	err := RunContext(
		context.Background(),
		[]string{"messages", "reply", messageURL, "--text", "hello", "--dry-run"},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Data plannedMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.ReplyTo == nil || result.Data.ReplyTo.URL != messageURL {
		t.Fatalf("reply reference = %#v", result.Data.ReplyTo)
	}
	if !result.Data.PingReplyAuthor {
		t.Fatal("reply author should be pinged by default")
	}
}

func TestMessageReplyCanSuppressMentions(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	err := RunContext(
		context.Background(),
		[]string{
			"messages", "reply", "223456789012345678/323456789012345678",
			"--text", "hello @everyone", "--no-mentions", "--no-ping-reply-author", "--dry-run",
		},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data plannedMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data.AllowedMentions) != 0 || result.Data.PingReplyAuthor {
		t.Fatalf("mentions were not suppressed: %#v", result.Data)
	}
}

func TestMessageContentUsesUTF16Limit(t *testing.T) {
	t.Parallel()

	_, err := messageContent(messageWriteOptions{Text: strings.Repeat("😀", 1001)}, strings.NewReader(""))
	if err == nil {
		t.Fatal("expected UTF-16 content limit error")
	}
}

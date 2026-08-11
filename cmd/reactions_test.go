package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestReactionAddDryRunWithExplicitIDs(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	err := RunContext(
		context.Background(),
		[]string{
			"reactions", "add",
			"--channel", "223456789012345678",
			"--message", "323456789012345678",
			"--emoji", "👍",
			"--dry-run",
		},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data reactionResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Message.ChannelID != "223456789012345678" || result.Data.Message.MessageID != "323456789012345678" {
		t.Fatalf("unexpected message identity: %#v", result.Data.Message)
	}
	if result.Data.Message.URL != "" {
		t.Fatalf("explicit IDs invented a URL: %q", result.Data.Message.URL)
	}
	if result.Data.Emoji != "👍" || result.Data.UnreadGuard != "enforced_at_reaction" {
		t.Fatalf("unexpected reaction plan: %#v", result.Data)
	}
}

func TestReactionAddDryRunAcceptsMessageURL(t *testing.T) {
	t.Parallel()

	const messageURL = "https://discord.com/channels/123456789012345678/223456789012345678/323456789012345678"
	var stdout, stderr bytes.Buffer
	err := RunContext(
		context.Background(),
		[]string{"reactions", "add", messageURL, "--emoji", "party:423456789012345678", "--ignore-unread", "--dry-run"},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
	)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data reactionResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Message.URL != messageURL || result.Data.Emoji != "party:423456789012345678" {
		t.Fatalf("unexpected reaction plan: %#v", result.Data)
	}
	if result.Data.UnreadGuard != "bypassed" {
		t.Fatalf("unread guard = %q", result.Data.UnreadGuard)
	}
}

func TestParseReactionEmojiDiscordMarkup(t *testing.T) {
	t.Parallel()

	emoji, err := parseReactionEmoji("<a:party:423456789012345678>")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(emoji); got != "party:423456789012345678" {
		t.Fatalf("emoji = %q", got)
	}
}

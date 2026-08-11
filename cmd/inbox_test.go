package cmd

import (
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/gateway"
	"github.com/ayn2op/arikawa/v3/utils/httputil"
)

func TestDeduplicateChannelReadStates(t *testing.T) {
	t.Parallel()

	channelID := discord.ChannelID(223456789012345678)
	states := deduplicateChannelReadStates([]gateway.ReadState{
		{ChannelID: channelID, MentionCount: 1},
		{ChannelID: discord.ChannelID(323456789012345678), Type: gateway.ReadStateTypeGuildEvent, MentionCount: 9},
		{ChannelID: channelID, MentionCount: 2},
	})
	if len(states) != 1 {
		t.Fatalf("got %d states, want 1", len(states))
	}
	if states[0].MentionCount != 2 {
		t.Fatalf("mention count = %d, want latest value 2", states[0].MentionCount)
	}
}

func TestClassifyInboxChannelError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status int
		code   string
	}{
		{status: 403, code: "missing_access"},
		{status: 404, code: "unknown_channel"},
	}
	for _, test := range tests {
		issue := classifyInboxChannelError(httputil.HTTPError{Status: test.status})
		if issue.Code != test.code {
			t.Fatalf("status %d classified as %q, want %q", test.status, issue.Code, test.code)
		}
	}
}

func TestUnavailableUnreadContext(t *testing.T) {
	t.Parallel()

	context := unavailableUnreadContext(gateway.ReadState{MentionCount: 3})
	if context.Available || context.Complete || !context.Truncated {
		t.Fatalf("unexpected unavailable context: %#v", context)
	}
	if context.ExpectedMentionCount != 3 || context.Messages == nil {
		t.Fatalf("context lost read-state metadata: %#v", context)
	}
}

func TestOrdinaryUnreadUsesCursorAndBadgeFallback(t *testing.T) {
	t.Parallel()

	state := gateway.ReadState{LastMessageID: discord.MessageID(100)}
	channel := discord.Channel{LastMessageID: discord.MessageID(101)}
	if !isOrdinaryUnread(state, &channel) {
		t.Fatal("newer channel message should be unread")
	}
	state.BadgeCount = 1
	if !isOrdinaryUnread(state, nil) {
		t.Fatal("badge count should preserve an unresolved unread entry")
	}
}

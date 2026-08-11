package discordclient

import (
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
)

func TestEvaluateChannelReadStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		latest     discord.MessageID
		cursor     discord.MessageID
		verifiable bool
		unread     bool
	}{
		{name: "empty channel", verifiable: true},
		{name: "missing cursor", latest: 200, verifiable: false},
		{name: "cursor behind", latest: 200, cursor: 199, verifiable: true, unread: true},
		{name: "cursor at latest", latest: 200, cursor: 200, verifiable: true},
		{name: "cursor beyond latest", latest: 200, cursor: 201, verifiable: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			status := evaluateChannelReadStatus(test.latest, test.cursor)
			if status.Verifiable != test.verifiable || status.Unread != test.unread {
				t.Fatalf("status = %#v, want verifiable=%v unread=%v", status, test.verifiable, test.unread)
			}
		})
	}
}

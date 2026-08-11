package cmd

import (
	"testing"

	"github.com/ayn2op/arikawa/v3/discord"
)

func TestParseMessageRef(t *testing.T) {
	t.Parallel()

	const (
		guildID   = "123456789012345678"
		channelID = "223456789012345678"
		messageID = "323456789012345678"
	)
	tests := []struct {
		name        string
		value       string
		channelHint discord.ChannelID
		wantGuild   string
		wantErr     bool
	}{
		{
			name:      "guild URL",
			value:     "https://discord.com/channels/" + guildID + "/" + channelID + "/" + messageID,
			wantGuild: guildID,
		},
		{
			name:  "DM URL",
			value: "https://discord.com/channels/@me/" + channelID + "/" + messageID,
		},
		{
			name:  "composite",
			value: channelID + "/" + messageID,
		},
		{
			name:        "bare with channel",
			value:       messageID,
			channelHint: discord.ChannelID(223456789012345678),
		},
		{
			name:    "bare without channel",
			value:   messageID,
			wantErr: true,
		},
		{
			name:    "untrusted URL",
			value:   "https://example.com/channels/" + guildID + "/" + channelID + "/" + messageID,
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ref, err := parseMessageRef(test.value, test.channelHint)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := ref.ChannelID.String(); got != channelID {
				t.Fatalf("channel ID = %q, want %q", got, channelID)
			}
			if got := ref.MessageID.String(); got != messageID {
				t.Fatalf("message ID = %q, want %q", got, messageID)
			}
			if test.wantGuild == "" {
				if ref.GuildID.IsValid() {
					t.Fatalf("unexpected guild ID %q", ref.GuildID)
				}
			} else if got := ref.GuildID.String(); got != test.wantGuild {
				t.Fatalf("guild ID = %q, want %q", got, test.wantGuild)
			}
		})
	}
}

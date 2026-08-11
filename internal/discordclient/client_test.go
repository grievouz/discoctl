package discordclient

import (
	"testing"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/gateway"
)

func TestDecodeVersionedReadyState(t *testing.T) {
	t.Parallel()

	ready := gateway.ReadyEvent{}
	ready.RawEventBody = []byte(`{
		"read_state":{"version":2,"entries":[{
			"id":"223456789012345678",
			"read_state_type":0,
			"last_message_id":"323456789012345678",
			"mention_count":4,
			"badge_count":5
		}]},
		"user_guild_settings":{"version":3,"entries":[{
			"guild_id":"123456789012345678",
			"muted":true,
			"mute_config":null,
			"channel_overrides":[{
				"channel_id":"223456789012345678",
				"muted":true,
				"mute_config":null
			}]
		}]}
	}`)

	readStates := decodeReadStates(&ready)
	if len(readStates) != 1 || readStates[0].MentionCount != 4 || readStates[0].BadgeCount != 5 {
		t.Fatalf("unexpected read states: %#v", readStates)
	}
	settings := decodeGuildSettings(&ready)
	if len(settings) != 1 || !settings[0].Muted || len(settings[0].ChannelOverrides) != 1 {
		t.Fatalf("unexpected guild settings: %#v", settings)
	}
}

func TestMuteActive(t *testing.T) {
	t.Parallel()

	if muteActive(false, nil) {
		t.Fatal("unmuted setting reported active")
	}
	if !muteActive(true, nil) {
		t.Fatal("permanent mute reported inactive")
	}
	if muteActive(true, &gateway.UserMuteConfig{
		SelectedTimeWindow: 60,
		EndTime:            discord.NewTimestamp(time.Now().Add(-time.Minute)),
	}) {
		t.Fatal("expired mute reported active")
	}
	if !muteActive(true, &gateway.UserMuteConfig{
		SelectedTimeWindow: 60,
		EndTime:            discord.NewTimestamp(time.Now().Add(time.Minute)),
	}) {
		t.Fatal("future mute reported inactive")
	}
}

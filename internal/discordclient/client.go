package discordclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/gateway"
	"github.com/ayn2op/arikawa/v3/session"
	"github.com/ayn2op/arikawa/v3/state"
	"github.com/ayn2op/arikawa/v3/state/store/defaultstore"
	"github.com/ayn2op/arikawa/v3/utils/handler"
	"github.com/ayn2op/arikawa/v3/utils/httputil"
	"github.com/ayn2op/ningen/v3"
	clientgateway "github.com/grievouz/discoctl/internal/gateway"
	clienthttp "github.com/grievouz/discoctl/internal/http"
)

type Client struct {
	State *ningen.State

	readStatesMu sync.RWMutex
	readStates   []gateway.ReadState

	settingsMu       sync.RWMutex
	guildSettings    map[discord.GuildID]gateway.UserGuildSetting
	channelOverrides map[discord.ChannelID]gateway.UserChannelOverride
}

func New(token string) *Client {
	identifier := gateway.DefaultIdentifier(token)
	identifier.Properties = clienthttp.IdentifyProperties()
	identifier.Capabilities = gateway.LazyUserNotes |
		gateway.VersionedReadStates |
		gateway.VersionedUserGuildSetttings |
		gateway.DedupeUserObjects |
		gateway.PrioritizedReadyPayload |
		gateway.MultipleGuildExperimentPopulations |
		gateway.NonChannelReadStates

	session := session.NewWithGateway(clientgateway.New(identifier), handler.New())
	session.Client = clienthttp.NewClient(token)
	baseState := state.NewFromSession(session, defaultstore.New())

	client := &Client{
		guildSettings:    make(map[discord.GuildID]gateway.UserGuildSetting),
		channelOverrides: make(map[discord.ChannelID]gateway.UserChannelOverride),
	}
	baseState.AddSyncHandler(client.captureReady)
	baseState.AddSyncHandler(client.captureGuildSettingsUpdate)

	client.State = ningen.FromState(baseState)
	client.State.StateLog = func(err error) {
		slog.Debug("Discord state", "err", err)
	}
	client.State.OnRequest = append(client.State.OnRequest, httputil.WithHeaders(clienthttp.Headers()))
	return client
}

func (c *Client) Open(ctx context.Context) error {
	if err := c.State.Open(ctx); err != nil {
		return fmt.Errorf("open Discord session: %w", err)
	}
	return nil
}

func (c *Client) Close() error {
	if err := c.State.Close(); err != nil {
		return fmt.Errorf("close Discord session: %w", err)
	}
	return nil
}

func (c *Client) ChannelReadStates() []gateway.ReadState {
	c.readStatesMu.RLock()
	defer c.readStatesMu.RUnlock()

	states := make([]gateway.ReadState, len(c.readStates))
	copy(states, c.readStates)
	return states
}

func (c *Client) captureReady(ready *gateway.ReadyEvent) {
	states := decodeReadStates(ready)
	c.readStatesMu.Lock()
	c.readStates = states
	c.readStatesMu.Unlock()

	c.replaceGuildSettings(decodeGuildSettings(ready))
}

func (c *Client) captureGuildSettingsUpdate(update *gateway.UserGuildSettingsUpdateEvent) {
	c.settingsMu.Lock()
	c.guildSettings[update.GuildID] = update.UserGuildSetting
	c.channelOverrides = make(map[discord.ChannelID]gateway.UserChannelOverride)
	for _, setting := range c.guildSettings {
		for _, override := range setting.ChannelOverrides {
			c.channelOverrides[override.ChannelID] = override
		}
	}
	c.settingsMu.Unlock()
}

func (c *Client) ChannelIsMuted(channel discord.Channel) bool {
	c.settingsMu.RLock()
	defer c.settingsMu.RUnlock()

	if override, ok := c.channelOverrides[channel.ID]; ok && muteActive(override.Muted, override.MuteConfig) {
		return true
	}
	if channel.ParentID.IsValid() {
		if override, ok := c.channelOverrides[channel.ParentID]; ok && muteActive(override.Muted, override.MuteConfig) {
			return true
		}
	}
	if setting, ok := c.guildSettings[channel.GuildID]; ok && muteActive(setting.Muted, setting.MuteConfig) {
		return true
	}
	return false
}

func decodeReadStates(ready *gateway.ReadyEvent) []gateway.ReadState {
	if len(ready.ReadStates) > 0 {
		states := make([]gateway.ReadState, len(ready.ReadStates))
		copy(states, ready.ReadStates)
		return states
	}

	var payload struct {
		ReadState json.RawMessage `json:"read_state"`
	}
	if len(ready.RawEventBody) == 0 || json.Unmarshal(ready.RawEventBody, &payload) != nil || len(payload.ReadState) == 0 {
		return nil
	}

	var states []gateway.ReadState
	if json.Unmarshal(payload.ReadState, &states) == nil {
		return states
	}

	var versioned struct {
		Entries []gateway.ReadState `json:"entries"`
	}
	if json.Unmarshal(payload.ReadState, &versioned) != nil {
		return nil
	}
	return versioned.Entries
}

func decodeGuildSettings(ready *gateway.ReadyEvent) []gateway.UserGuildSetting {
	if len(ready.UserGuildSettings) > 0 {
		settings := make([]gateway.UserGuildSetting, len(ready.UserGuildSettings))
		copy(settings, ready.UserGuildSettings)
		return settings
	}

	var payload struct {
		Settings json.RawMessage `json:"user_guild_settings"`
	}
	if len(ready.RawEventBody) == 0 || json.Unmarshal(ready.RawEventBody, &payload) != nil || len(payload.Settings) == 0 {
		return nil
	}

	var settings []gateway.UserGuildSetting
	if json.Unmarshal(payload.Settings, &settings) == nil {
		return settings
	}
	var versioned struct {
		Entries []gateway.UserGuildSetting `json:"entries"`
	}
	if json.Unmarshal(payload.Settings, &versioned) != nil {
		return nil
	}
	return versioned.Entries
}

func (c *Client) replaceGuildSettings(settings []gateway.UserGuildSetting) {
	guilds := make(map[discord.GuildID]gateway.UserGuildSetting, len(settings))
	channels := make(map[discord.ChannelID]gateway.UserChannelOverride)
	for _, setting := range settings {
		guilds[setting.GuildID] = setting
		for _, override := range setting.ChannelOverrides {
			channels[override.ChannelID] = override
		}
	}
	c.settingsMu.Lock()
	c.guildSettings = guilds
	c.channelOverrides = channels
	c.settingsMu.Unlock()
}

func muteActive(muted bool, config *gateway.UserMuteConfig) bool {
	if !muted {
		return false
	}
	if config == nil || config.SelectedTimeWindow == -1 {
		return true
	}
	return config.EndTime.Time().After(time.Now())
}

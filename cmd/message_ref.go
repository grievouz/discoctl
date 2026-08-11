package cmd

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/ayn2op/arikawa/v3/discord"
)

type messageRef struct {
	GuildID   discord.GuildID
	ChannelID discord.ChannelID
	MessageID discord.MessageID
	URL       string
}

type messageRefView struct {
	GuildID   *string `json:"guild_id"`
	ChannelID string  `json:"channel_id"`
	MessageID string  `json:"message_id"`
	URL       string  `json:"url,omitempty"`
}

func parseMessageRef(value string, channelHint discord.ChannelID) (messageRef, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return messageRef{}, errors.New("message reference is empty")
	}

	if strings.Contains(value, "://") {
		return parseMessageURL(value, channelHint)
	}
	parts := strings.Split(value, "/")
	switch len(parts) {
	case 1:
		if !channelHint.IsValid() {
			return messageRef{}, errors.New("a bare message ID requires --channel <channel-id>")
		}
		messageID, err := parseMessageID(parts[0])
		if err != nil {
			return messageRef{}, err
		}
		return messageRef{ChannelID: channelHint, MessageID: messageID}, nil
	case 2:
		channelID, err := parseChannelID(parts[0])
		if err != nil {
			return messageRef{}, err
		}
		if channelHint.IsValid() && channelHint != channelID {
			return messageRef{}, errors.New("message reference channel does not match --channel")
		}
		messageID, err := parseMessageID(parts[1])
		if err != nil {
			return messageRef{}, err
		}
		return messageRef{ChannelID: channelID, MessageID: messageID}, nil
	default:
		return messageRef{}, errors.New("message reference must be a Discord message URL, channelID/messageID, or a message ID with --channel")
	}
}

func parseMessageURL(value string, channelHint discord.ChannelID) (messageRef, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return messageRef{}, fmt.Errorf("invalid message URL: %w", err)
	}
	host := strings.ToLower(parsed.Hostname())
	switch host {
	case "discord.com", "www.discord.com", "canary.discord.com", "ptb.discord.com", "discordapp.com":
	default:
		return messageRef{}, fmt.Errorf("unsupported message URL host %q", host)
	}
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) != 4 || parts[0] != "channels" {
		return messageRef{}, errors.New("Discord message URL must end in /channels/<guild-or-@me>/<channel>/<message>")
	}

	channelID, err := parseChannelID(parts[2])
	if err != nil {
		return messageRef{}, err
	}
	if channelHint.IsValid() && channelHint != channelID {
		return messageRef{}, errors.New("message URL channel does not match --channel")
	}
	messageID, err := parseMessageID(parts[3])
	if err != nil {
		return messageRef{}, err
	}
	ref := messageRef{ChannelID: channelID, MessageID: messageID}
	if parts[1] != "@me" {
		ref.GuildID, err = parseGuildID(parts[1])
		if err != nil {
			return messageRef{}, err
		}
	}
	ref.URL = messageURL(ref.GuildID, ref.ChannelID, ref.MessageID)
	return ref, nil
}

func (ref messageRef) view() messageRefView {
	view := messageRefView{
		ChannelID: ref.ChannelID.String(),
		MessageID: ref.MessageID.String(),
		URL:       ref.URL,
	}
	if ref.GuildID.IsValid() {
		guildID := ref.GuildID.String()
		view.GuildID = &guildID
	}
	return view
}

func messageURL(guildID discord.GuildID, channelID discord.ChannelID, messageID discord.MessageID) string {
	guild := "@me"
	if guildID.IsValid() {
		guild = guildID.String()
	}
	return fmt.Sprintf("https://discord.com/channels/%s/%s/%s", guild, channelID, messageID)
}

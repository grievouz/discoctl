package cmd

import (
	"fmt"

	"github.com/ayn2op/arikawa/v3/discord"
)

func parseGuildID(value string) (discord.GuildID, error) {
	snowflake, err := discord.ParseSnowflake(value)
	if err != nil {
		return 0, fmt.Errorf("invalid guild ID %q: %w", value, err)
	}
	return discord.GuildID(snowflake), nil
}

func parseChannelID(value string) (discord.ChannelID, error) {
	snowflake, err := discord.ParseSnowflake(value)
	if err != nil {
		return 0, fmt.Errorf("invalid channel ID %q: %w", value, err)
	}
	return discord.ChannelID(snowflake), nil
}

func parseMessageID(value string) (discord.MessageID, error) {
	snowflake, err := discord.ParseSnowflake(value)
	if err != nil {
		return 0, fmt.Errorf("invalid message ID %q: %w", value, err)
	}
	return discord.MessageID(snowflake), nil
}

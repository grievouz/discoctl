package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/grievouz/discoctl/internal/discordclient"
)

func runChannels(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printChannelsUsage(stdout)
		return nil
	}
	if args[0] != "list" {
		return invalidArgumentsf("unknown channels command %q; run 'discoctl channels help'", args[0])
	}

	flags := flag.NewFlagSet("channels list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	guildValue := flags.String("guild", "", "guild ID")
	output := addJSONFlag(flags)
	if err := parseFlags(flags, args[1:]); err != nil {
		return err
	}
	if err := requireNoPositionals(flags); err != nil {
		return fmt.Errorf("channels list: %w", err)
	}
	if *guildValue == "" {
		return invalidArguments(errors.New("channels list requires --guild <guild-id>"))
	}
	guildID, err := parseGuildID(*guildValue)
	if err != nil {
		return err
	}

	return withDiscordClient(ctx, func(client *discordclient.Client) error {
		channels, err := client.State.State.Channels(guildID)
		if err != nil {
			return fmt.Errorf("list channels: %w", err)
		}
		sort.Slice(channels, func(i, j int) bool {
			if channels[i].Position == channels[j].Position {
				return channels[i].ID < channels[j].ID
			}
			return channels[i].Position < channels[j].Position
		})

		views := make([]channelView, len(channels))
		for i, channel := range channels {
			views[i] = newChannelView(channel)
		}
		return output.writeJSON(stdout, views, nil, nil)
	})
}

func printChannelsUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: discoctl channels list --guild <guild-id> [--pretty] [--json]

Lists the channels in one guild. Use the returned channel IDs with messages commands.`)
}

package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/grievouz/discoctl/internal/discordclient"
)

func runGuilds(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printGuildsUsage(stdout)
		return nil
	}
	if args[0] != "list" {
		return fmt.Errorf("unknown guilds command %q; run 'discoctl guilds help'", args[0])
	}

	flags := flag.NewFlagSet("guilds list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	addJSONFlag(flags)
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if err := requireNoPositionals(flags); err != nil {
		return fmt.Errorf("guilds list: %w", err)
	}

	return withDiscordClient(ctx, func(client *discordclient.Client) error {
		guilds, err := client.State.Guilds()
		if err != nil {
			return fmt.Errorf("list guilds: %w", err)
		}
		sort.Slice(guilds, func(i, j int) bool {
			if guilds[i].Name == guilds[j].Name {
				return guilds[i].ID < guilds[j].ID
			}
			return guilds[i].Name < guilds[j].Name
		})

		views := make([]guildView, len(guilds))
		for i, guild := range guilds {
			views[i] = newGuildView(guild)
		}
		return writeJSON(stdout, views, nil, nil)
	})
}

func printGuildsUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: discoctl guilds list [--json]

Lists guilds visible to the authenticated profile. IDs are serialized as strings.`)
}

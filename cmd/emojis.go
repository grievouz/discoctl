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

type emojiView struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Syntax    string   `json:"syntax"`
	Animated  bool     `json:"animated"`
	Available bool     `json:"available"`
	Managed   bool     `json:"managed"`
	RoleIDs   []string `json:"role_ids"`
	URL       string   `json:"url"`
}

func runEmojis(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printEmojisUsage(stdout)
		return nil
	}
	if args[0] != "list" {
		return invalidArgumentsf("unknown emojis command %q; run 'discoctl emojis help'", args[0])
	}

	flags := flag.NewFlagSet("emojis list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	guildValue := flags.String("guild", "", "guild ID")
	output := addJSONFlag(flags)
	if err := parseFlags(flags, args[1:]); err != nil {
		return err
	}
	if err := requireNoPositionals(flags); err != nil {
		return fmt.Errorf("emojis list: %w", err)
	}
	if *guildValue == "" {
		return invalidArguments(errors.New("emojis list requires --guild <guild-id>"))
	}
	guildID, err := parseGuildID(*guildValue)
	if err != nil {
		return err
	}

	return withDiscordClient(ctx, func(client *discordclient.Client) error {
		emojis, err := client.State.Emojis(guildID)
		if err != nil {
			return fmt.Errorf("list emojis: %w", err)
		}
		sort.Slice(emojis, func(i, j int) bool {
			if emojis[i].Name == emojis[j].Name {
				return emojis[i].ID < emojis[j].ID
			}
			return emojis[i].Name < emojis[j].Name
		})
		views := make([]emojiView, len(emojis))
		for i, emoji := range emojis {
			roleIDs := make([]string, len(emoji.RoleIDs))
			for j, roleID := range emoji.RoleIDs {
				roleIDs[j] = roleID.String()
			}
			views[i] = emojiView{
				ID:        emoji.ID.String(),
				Name:      emoji.Name,
				Syntax:    emoji.String(),
				Animated:  emoji.Animated,
				Available: emoji.Available,
				Managed:   emoji.Managed,
				RoleIDs:   roleIDs,
				URL:       emoji.EmojiURL(),
			}
		}
		return output.writeJSON(stdout, views, nil, nil)
	})
}

func printEmojisUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: discoctl emojis list --guild <guild-id> [--pretty] [--json]

Lists custom guild emojis and the exact syntax accepted in message text.`)
}

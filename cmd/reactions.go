package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/grievouz/discoctl/internal/discordclient"
)

type reactionResult struct {
	Action      string         `json:"action"`
	Applied     bool           `json:"applied"`
	DryRun      bool           `json:"dry_run"`
	Message     messageRefView `json:"message"`
	Emoji       string         `json:"emoji"`
	UnreadGuard string         `json:"unread_guard"`
}

func runReactions(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printReactionsUsage(stdout)
		return nil
	}
	if args[0] != "add" {
		return invalidArgumentsf("unknown reactions command %q; run 'discoctl reactions help'", args[0])
	}
	return runReactionsAdd(ctx, args[1:], stdout, stderr)
}

func runReactionsAdd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if hasHelpArg(args) {
		printReactionsUsage(stdout)
		return nil
	}
	referenceValue, args, err := takeLeadingReference(args)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("reactions add", flag.ContinueOnError)
	flags.SetOutput(stderr)
	channelValue := flags.String("channel", "", "channel ID")
	messageValue := flags.String("message", "", "message ID")
	emojiValue := flags.String("emoji", "", "Unicode emoji or custom name:emoji-id")
	ignoreUnread := flags.Bool("ignore-unread", false, "react even when the channel has unread messages or its read state is unknown")
	dryRun := flags.Bool("dry-run", false, "validate and print the operation without connecting to Discord")
	output := addJSONFlag(flags)
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if flags.NArg() > 1 || (flags.NArg() == 1 && referenceValue != "") {
		return invalidArguments(errors.New("reactions add accepts at most one message reference"))
	}
	if flags.NArg() == 1 {
		referenceValue = flags.Arg(0)
	}

	ref, err := resolveReactionMessage(referenceValue, *channelValue, *messageValue)
	if err != nil {
		return err
	}
	emoji, err := parseReactionEmoji(*emojiValue)
	if err != nil {
		return err
	}
	guard := "enforced_at_reaction"
	if *ignoreUnread {
		guard = "bypassed"
	}
	result := reactionResult{
		Action:      "add_reaction",
		DryRun:      *dryRun,
		Message:     ref.view(),
		Emoji:       string(emoji),
		UnreadGuard: guard,
	}
	if *dryRun {
		return output.writeJSON(stdout, result, nil, nil)
	}

	return withDiscordClient(ctx, func(client *discordclient.Client) error {
		if !*ignoreUnread {
			if err := enforceUnreadGuard(client, ref.ChannelID); err != nil {
				return err
			}
		}
		if err := client.State.Session.React(ref.ChannelID, ref.MessageID, emoji); err != nil {
			return fmt.Errorf("add reaction: %w", err)
		}
		result.Applied = true
		return output.writeJSON(stdout, result, nil, nil)
	})
}

func resolveReactionMessage(referenceValue, channelValue, messageValue string) (messageRef, error) {
	if referenceValue != "" && (channelValue != "" || messageValue != "") {
		return messageRef{}, invalidArguments(errors.New("use either a message reference or --channel and --message, not both"))
	}
	if referenceValue != "" {
		return parseMessageRef(referenceValue, 0)
	}
	if channelValue == "" || messageValue == "" {
		return messageRef{}, invalidArguments(errors.New("reactions add requires --channel <channel-id> and --message <message-id>"))
	}
	channelID, err := parseChannelID(channelValue)
	if err != nil {
		return messageRef{}, err
	}
	return parseMessageRef(messageValue, channelID)
}

func parseReactionEmoji(value string) (discord.APIEmoji, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", invalidArguments(errors.New("reactions add requires --emoji <emoji>"))
	}

	custom := value
	if strings.HasPrefix(custom, "<") && strings.HasSuffix(custom, ">") {
		custom = strings.TrimSuffix(strings.TrimPrefix(custom, "<"), ">")
		custom = strings.TrimPrefix(custom, "a:")
		custom = strings.TrimPrefix(custom, ":")
	}
	if strings.Count(custom, ":") == 1 {
		parts := strings.SplitN(custom, ":", 2)
		if parts[0] == "" {
			return "", invalidArguments(errors.New("custom emoji name is empty"))
		}
		snowflake, err := discord.ParseSnowflake(parts[1])
		if err != nil {
			return "", invalidArgumentsf("invalid custom emoji ID %q: %w", parts[1], err)
		}
		return discord.NewAPIEmoji(discord.EmojiID(snowflake), parts[0]), nil
	}
	if strings.Contains(custom, ":") {
		return "", invalidArguments(errors.New("custom emoji must use name:emoji-id or Discord's <:name:emoji-id> syntax"))
	}
	return discord.APIEmoji(value), nil
}

func printReactionsUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  discoctl reactions add --channel <channel-id> --message <message-id>
      --emoji <unicode-or-name:id> [--ignore-unread] [--dry-run] [--pretty] [--json]

A Discord message URL or channelID/messageID may be supplied positionally as a
convenience. Explicit IDs are the canonical interface. The unread guard is
enforced by default.`)
}

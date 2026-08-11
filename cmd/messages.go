package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/grievouz/discoctl/internal/discordclient"
)

const (
	defaultMessageLimit = 50
	maxMessageLimit     = 100
)

type messagePage struct {
	Limit       int     `json:"limit"`
	Order       string  `json:"order"`
	HasMore     *bool   `json:"has_more,omitempty"`
	MayHaveMore bool    `json:"may_have_more,omitempty"`
	OldestID    *string `json:"oldest_id"`
	NewestID    *string `json:"newest_id"`
	NextBefore  *string `json:"next_before"`
	NextAfter   *string `json:"next_after"`
}

func runMessages(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printMessagesUsage(stdout)
		return nil
	}

	switch args[0] {
	case "list":
		return runMessagesList(ctx, args[1:], stdout, stderr)
	case "get":
		return runMessagesGet(ctx, args[1:], stdout, stderr)
	case "send":
		return runMessagesSend(ctx, args[1:], stdin, stdout, stderr)
	case "reply":
		return runMessagesReply(ctx, args[1:], stdin, stdout, stderr)
	default:
		return fmt.Errorf("unknown messages command %q; run 'discoctl messages help'", args[0])
	}
}

func runMessagesList(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("messages list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	channelValue := flags.String("channel", "", "channel ID")
	beforeValue := flags.String("before", "", "return messages before this message ID")
	afterValue := flags.String("after", "", "return messages after this message ID")
	aroundValue := flags.String("around", "", "return messages around this message ID")
	limit := flags.Int("limit", defaultMessageLimit, "maximum number of messages (1-100)")
	addJSONFlag(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := requireNoPositionals(flags); err != nil {
		return fmt.Errorf("messages list: %w", err)
	}
	if *channelValue == "" {
		return errors.New("messages list requires --channel <channel-id>")
	}
	if *limit < 1 || *limit > maxMessageLimit {
		return fmt.Errorf("messages list --limit must be between 1 and %d", maxMessageLimit)
	}
	if setCount(*beforeValue, *afterValue, *aroundValue) > 1 {
		return errors.New("messages list accepts only one of --before, --after, or --around")
	}

	channelID, err := parseChannelID(*channelValue)
	if err != nil {
		return err
	}
	before, err := optionalMessageID(*beforeValue)
	if err != nil {
		return err
	}
	after, err := optionalMessageID(*afterValue)
	if err != nil {
		return err
	}
	around, err := optionalMessageID(*aroundValue)
	if err != nil {
		return err
	}

	return withDiscordClient(ctx, func(client *discordclient.Client) error {
		messages, hasMore, factual, err := fetchMessagePage(client, channelID, before, after, around, *limit)
		if err != nil {
			return fmt.Errorf("list messages: %w", err)
		}
		views := make([]messageView, len(messages))
		for i, message := range messages {
			views[i] = newMessageView(message)
		}
		page := newMessagePage(messages, *limit, hasMore, factual)
		return writeJSON(stdout, views, page, nil)
	})
}

func runMessagesGet(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("messages get", flag.ContinueOnError)
	flags.SetOutput(stderr)
	channelValue := flags.String("channel", "", "channel ID")
	messageValue := flags.String("message", "", "message ID")
	addJSONFlag(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := requireNoPositionals(flags); err != nil {
		return fmt.Errorf("messages get: %w", err)
	}
	if *channelValue == "" || *messageValue == "" {
		return errors.New("messages get requires --channel <channel-id> and --message <message-id>")
	}
	channelID, err := parseChannelID(*channelValue)
	if err != nil {
		return err
	}
	messageID, err := parseMessageID(*messageValue)
	if err != nil {
		return err
	}

	return withDiscordClient(ctx, func(client *discordclient.Client) error {
		message, err := client.State.Session.Message(channelID, messageID)
		if err != nil {
			return fmt.Errorf("get message: %w", err)
		}
		return writeJSON(stdout, newMessageView(*message), nil, nil)
	})
}

func fetchMessagePage(client *discordclient.Client, channelID discord.ChannelID, before, after, around discord.MessageID, limit int) ([]discord.Message, bool, bool, error) {
	requestLimit := uint(limit + 1)
	var (
		messages []discord.Message
		err      error
	)

	switch {
	case around.IsValid():
		messages, err = client.State.Session.MessagesAround(channelID, around, uint(limit))
		if err != nil {
			return nil, false, false, err
		}
		slices.Reverse(messages)
		return messages, false, false, nil
	case after.IsValid():
		messages, err = client.State.Session.MessagesAfter(channelID, after, requestLimit)
		if err != nil {
			return nil, false, true, err
		}
		hasMore := len(messages) > limit
		if hasMore {
			messages = messages[1:]
		}
		slices.Reverse(messages)
		return messages, hasMore, true, nil
	default:
		messages, err = client.State.Session.MessagesBefore(channelID, before, requestLimit)
		if err != nil {
			return nil, false, true, err
		}
		hasMore := len(messages) > limit
		if hasMore {
			messages = messages[:limit]
		}
		slices.Reverse(messages)
		return messages, hasMore, true, nil
	}
}

func fetchAfterAscending(client *discordclient.Client, channelID discord.ChannelID, after discord.MessageID, limit int) ([]discord.Message, bool, error) {
	messages, hasMore, _, err := fetchMessagePage(client, channelID, 0, after, 0, limit)
	return messages, hasMore, err
}

func newMessagePage(messages []discord.Message, limit int, hasMore, factual bool) messagePage {
	page := messagePage{Limit: limit, Order: "oldest_to_newest"}
	if factual {
		page.HasMore = &hasMore
	} else {
		page.MayHaveMore = len(messages) == limit
	}
	if len(messages) == 0 {
		return page
	}
	oldest := messages[0].ID.String()
	newest := messages[len(messages)-1].ID.String()
	page.OldestID = &oldest
	page.NewestID = &newest
	page.NextBefore = &oldest
	page.NextAfter = &newest
	return page
}

func optionalMessageID(value string) (discord.MessageID, error) {
	if value == "" {
		return 0, nil
	}
	return parseMessageID(value)
}

func setCount(values ...string) int {
	count := 0
	for _, value := range values {
		if value != "" {
			count++
		}
	}
	return count
}

func printMessagesUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  discoctl messages list --channel <channel-id> [--limit 50]
      [--before <message-id> | --after <message-id> | --around <message-id>] [--json]
  discoctl messages get --channel <channel-id> --message <message-id> [--json]

Run 'discoctl messages send --help' or 'discoctl messages reply --help' for
the state-changing commands.

History is returned oldest-to-newest and fetching it never marks messages read.`)
}

package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"

	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/utils/json/option"
	"github.com/grievouz/discoctl/internal/discordclient"
)

const maxMessageInputBytes = 16 << 10

type plannedMessage struct {
	Action          string          `json:"action"`
	DryRun          bool            `json:"dry_run"`
	ChannelID       string          `json:"channel_id"`
	Content         string          `json:"content"`
	Nonce           string          `json:"nonce,omitempty"`
	ReplyTo         *messageRefView `json:"reply_to,omitempty"`
	AllowedMentions []string        `json:"allowed_mentions"`
	PingReplyAuthor bool            `json:"ping_reply_author"`
	UnreadGuard     string          `json:"unread_guard"`
}

type messageWriteOptions struct {
	ChannelValue   string
	MessageValue   string
	ReferenceValue string
	Text           string
	MessageAlias   string
	ReadStdin      bool
	DryRun         bool
	Nonce          string
	NoMentions     bool
	NoUserMentions bool
	NoRoleMentions bool
	NoEveryone     bool
	NoPingReply    bool
	IgnoreUnread   bool
}

func runMessagesSend(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if hasHelpArg(args) {
		printMessagesSendUsage(stdout)
		return nil
	}
	flags := flag.NewFlagSet("messages send", flag.ContinueOnError)
	flags.SetOutput(stderr)
	options := addMessageWriteFlags(flags)
	replyValue := flags.String("reply", "", "reply to a message URL, channelID/messageID, or message ID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := requireNoPositionals(flags); err != nil {
		return fmt.Errorf("messages send: %w", err)
	}
	options.ReferenceValue = *replyValue
	return executeMessageWrite(ctx, *options, stdin, stdout)
}

func runMessagesReply(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if hasHelpArg(args) {
		printMessagesReplyUsage(stdout)
		return nil
	}
	referenceValue, args, err := takeLeadingReference(args)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("messages reply", flag.ContinueOnError)
	flags.SetOutput(stderr)
	options := addMessageWriteFlags(flags)
	messageValue := flags.String("message", "", "message ID (requires --channel)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 || (flags.NArg() == 1 && referenceValue != "") {
		return errors.New("messages reply accepts one message reference")
	}
	if flags.NArg() == 1 {
		referenceValue = flags.Arg(0)
	}
	options.ReferenceValue = referenceValue
	options.MessageValue = *messageValue
	if options.ReferenceValue == "" && options.MessageValue == "" {
		return errors.New("messages reply requires a message reference or --channel and --message")
	}
	return executeMessageWrite(ctx, *options, stdin, stdout)
}

func addMessageWriteFlags(flags *flag.FlagSet) *messageWriteOptions {
	options := &messageWriteOptions{}
	flags.StringVar(&options.ChannelValue, "channel", "", "channel ID")
	flags.StringVar(&options.Text, "text", "", "message text")
	flags.StringVar(&options.MessageAlias, "msg", "", "alias for --text")
	flags.BoolVar(&options.ReadStdin, "stdin", false, "read message text from standard input")
	flags.BoolVar(&options.DryRun, "dry-run", false, "validate and print the payload without connecting to Discord")
	flags.StringVar(&options.Nonce, "nonce", "", "client nonce for retry deduplication")
	flags.BoolVar(&options.NoMentions, "no-mentions", false, "disable all mention parsing")
	flags.BoolVar(&options.NoUserMentions, "no-user-mentions", false, "disable user mention parsing")
	flags.BoolVar(&options.NoRoleMentions, "no-role-mentions", false, "disable role mention parsing")
	flags.BoolVar(&options.NoEveryone, "no-everyone", false, "disable @everyone and @here parsing")
	flags.BoolVar(&options.NoPingReply, "no-ping-reply-author", false, "do not notify the author of a replied-to message")
	flags.BoolVar(&options.IgnoreUnread, "ignore-unread", false, "send even when the channel has unread messages or its read state is unknown")
	addJSONFlag(flags)
	return options
}

func executeMessageWrite(ctx context.Context, options messageWriteOptions, stdin io.Reader, stdout io.Writer) error {
	channelID, err := optionalChannelID(options.ChannelValue)
	if err != nil {
		return err
	}

	reference, err := resolveWriteReference(options, channelID)
	if err != nil {
		return err
	}
	if !channelID.IsValid() && reference != nil {
		channelID = reference.ChannelID
	}
	if !channelID.IsValid() {
		return errors.New("message send requires --channel <channel-id>")
	}

	content, err := messageContent(options, stdin)
	if err != nil {
		return err
	}
	allowedMentions, allowedNames := makeAllowedMentions(options)
	data := api.SendMessageData{
		Content:         content,
		Nonce:           options.Nonce,
		AllowedMentions: allowedMentions,
	}
	if reference != nil {
		data.Reference = &discord.MessageReference{
			MessageID: reference.MessageID,
			ChannelID: reference.ChannelID,
			GuildID:   reference.GuildID,
		}
	}

	planned := plannedMessage{
		Action:          "send_message",
		DryRun:          options.DryRun,
		ChannelID:       channelID.String(),
		Content:         content,
		Nonce:           options.Nonce,
		AllowedMentions: allowedNames,
		PingReplyAuthor: reference != nil && !options.NoPingReply,
		UnreadGuard:     "enforced_at_send",
	}
	if options.IgnoreUnread {
		planned.UnreadGuard = "bypassed"
	}
	if reference != nil {
		view := reference.view()
		planned.ReplyTo = &view
	}
	if options.DryRun {
		return writeJSON(stdout, planned, nil, nil)
	}

	return withDiscordClient(ctx, func(client *discordclient.Client) error {
		if !options.IgnoreUnread {
			if err := enforceUnreadGuard(client, channelID); err != nil {
				return err
			}
		}
		message, err := client.State.Session.SendMessageComplex(channelID, data)
		if err != nil {
			return fmt.Errorf("send message: %w", err)
		}
		return writeJSON(stdout, newMessageView(*message), nil, nil)
	})
}

func enforceUnreadGuard(client *discordclient.Client, channelID discord.ChannelID) error {
	status, err := client.ChannelReadStatus(channelID)
	if err != nil {
		return fmt.Errorf("check unread guard: %w; use --ignore-unread to send anyway", err)
	}
	if !status.Verifiable {
		return fmt.Errorf(
			"channel %s read state cannot be verified (latest message %s, no read cursor); use --ignore-unread to send anyway",
			channelID,
			status.LatestMessageID,
		)
	}
	if status.Unread {
		return fmt.Errorf(
			"channel %s has unread messages (read cursor %s, latest message %s); use --ignore-unread to send anyway",
			channelID,
			status.ReadCursor,
			status.LatestMessageID,
		)
	}
	return nil
}

func resolveWriteReference(options messageWriteOptions, channelHint discord.ChannelID) (*messageRef, error) {
	if options.ReferenceValue != "" && options.MessageValue != "" {
		return nil, errors.New("use either a message reference or --message, not both")
	}
	value := options.ReferenceValue
	if value == "" {
		value = options.MessageValue
	}
	if value == "" {
		return nil, nil
	}
	reference, err := parseMessageRef(value, channelHint)
	if err != nil {
		return nil, err
	}
	return &reference, nil
}

func messageContent(options messageWriteOptions, stdin io.Reader) (string, error) {
	sources := 0
	if options.Text != "" {
		sources++
	}
	if options.MessageAlias != "" {
		sources++
	}
	if options.ReadStdin {
		sources++
	}
	if sources != 1 {
		return "", errors.New("provide exactly one of --text, --msg, or --stdin")
	}

	content := options.Text
	if options.MessageAlias != "" {
		content = options.MessageAlias
	}
	if options.ReadStdin {
		body, err := io.ReadAll(io.LimitReader(stdin, maxMessageInputBytes+1))
		if err != nil {
			return "", fmt.Errorf("read message from stdin: %w", err)
		}
		if len(body) > maxMessageInputBytes {
			return "", errors.New("message input exceeds maximum size")
		}
		content = strings.TrimSuffix(string(body), "\n")
	}
	if content == "" {
		return "", errors.New("message text is empty")
	}
	if utf16Length(content) > 2000 {
		return "", errors.New("message text exceeds Discord's 2000-character limit")
	}
	return content, nil
}

func makeAllowedMentions(options messageWriteOptions) (*api.AllowedMentions, []string) {
	parse := make([]api.AllowedMentionType, 0, 3)
	names := make([]string, 0, 3)
	if !options.NoMentions && !options.NoUserMentions {
		parse = append(parse, api.AllowUserMention)
		names = append(names, "users")
	}
	if !options.NoMentions && !options.NoRoleMentions {
		parse = append(parse, api.AllowRoleMention)
		names = append(names, "roles")
	}
	if !options.NoMentions && !options.NoEveryone {
		parse = append(parse, api.AllowEveryoneMention)
		names = append(names, "everyone")
	}
	repliedUser := option.True
	if options.NoPingReply {
		repliedUser = option.False
	}
	return &api.AllowedMentions{Parse: parse, RepliedUser: repliedUser}, names
}

func optionalChannelID(value string) (discord.ChannelID, error) {
	if value == "" {
		return 0, nil
	}
	return parseChannelID(value)
}

func utf16Length(value string) int {
	return len(utf16.Encode([]rune(value)))
}

func hasHelpArg(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			return true
		}
	}
	return false
}

func takeLeadingReference(args []string) (string, []string, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", args, nil
	}
	if args[0] == "help" {
		return "", nil, nil
	}
	return args[0], args[1:], nil
}

func printMessagesSendUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: discoctl messages send --channel <channel-id>
      (--text <message> | --msg <message> | --stdin) [--reply <message-ref>]
      [--nonce <nonce>] [--ignore-unread] [--dry-run] [mention options] [--json]

Mentions are parsed by default. Use --no-mentions or one of the granular
--no-*-mentions flags to suppress them.`)
}

func printMessagesReplyUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  discoctl messages reply --channel <channel-id> --message <message-id>
      (--text <message> | --msg <message> | --stdin) [--nonce <nonce>]
      [--ignore-unread] [--dry-run] [mention options] [--json]

  discoctl messages reply <message-ref> (--text <message> | --msg <message> | --stdin)

Explicit channel and message IDs are the canonical interface. A message URL or
channelID/messageID reference is accepted as a convenience. Replies ping the
author by default; use --no-ping-reply-author to suppress that notification.`)
}

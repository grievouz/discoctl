package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/gateway"
	"github.com/ayn2op/arikawa/v3/utils/httputil"
	"github.com/grievouz/discoctl/internal/discordclient"
)

const defaultInboxMessageLimit = 20

type inboxEntry struct {
	ChannelID        string         `json:"channel_id"`
	GuildID          *string        `json:"guild_id"`
	Channel          *channelView   `json:"channel,omitempty"`
	ReadCursor       *string        `json:"read_cursor"`
	LatestMessage    *string        `json:"latest_message_id"`
	MentionCount     int            `json:"mention_count"`
	BadgeCount       int            `json:"badge_count"`
	Unread           bool           `json:"unread"`
	Muted            bool           `json:"muted"`
	AttentionCause   string         `json:"attention_cause"`
	ChannelAvailable bool           `json:"channel_available"`
	Error            *inboxError    `json:"error,omitempty"`
	Context          *unreadContext `json:"context,omitempty"`
}

type inboxError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type unreadContext struct {
	Kind                    string        `json:"kind"`
	ExpectedMentionCount    int           `json:"expected_mention_count"`
	ExactMentionsIdentified bool          `json:"exact_mentions_identified"`
	Available               bool          `json:"available"`
	Complete                bool          `json:"complete"`
	Truncated               bool          `json:"truncated"`
	Messages                []messageView `json:"messages"`
}

type inboxPage struct {
	Mode             string `json:"mode"`
	IncludeMuted     bool   `json:"include_muted"`
	MessagesIncluded bool   `json:"messages_included"`
	EntryCount       int    `json:"entry_count"`
}

func runInbox(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printInboxUsage(stdout)
		return nil
	}
	if args[0] != "unread" {
		return fmt.Errorf("unknown inbox command %q; run 'discoctl inbox help'", args[0])
	}

	flags := flag.NewFlagSet("inbox unread", flag.ContinueOnError)
	flags.SetOutput(stderr)
	includeAll := flags.Bool("all", false, "include ordinary unread channels in addition to mentions")
	includeMuted := flags.Bool("include-muted", false, "include muted ordinary unread channels")
	includeMessages := flags.Bool("messages", false, "include bounded unread message context")
	limitPerChannel := flags.Int("limit-per-channel", defaultInboxMessageLimit, "maximum context messages per channel (1-100)")
	addJSONFlag(flags)
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if err := requireNoPositionals(flags); err != nil {
		return fmt.Errorf("inbox unread: %w", err)
	}
	if *includeMuted && !*includeAll {
		return errors.New("inbox unread --include-muted requires --all")
	}
	if *limitPerChannel < 1 || *limitPerChannel > maxMessageLimit {
		return fmt.Errorf("inbox unread --limit-per-channel must be between 1 and %d", maxMessageLimit)
	}

	return withDiscordClient(ctx, func(client *discordclient.Client) error {
		entries, warnings := collectInbox(client, inboxOptions{
			IncludeAll:      *includeAll,
			IncludeMuted:    *includeMuted,
			IncludeMessages: *includeMessages,
			LimitPerChannel: *limitPerChannel,
		})
		mode := "mentions"
		if *includeAll {
			mode = "all_unread"
		}
		return writeJSON(stdout, entries, inboxPage{
			Mode:             mode,
			IncludeMuted:     *includeMuted,
			MessagesIncluded: *includeMessages,
			EntryCount:       len(entries),
		}, warnings)
	})
}

type inboxOptions struct {
	IncludeAll      bool
	IncludeMuted    bool
	IncludeMessages bool
	LimitPerChannel int
}

func collectInbox(client *discordclient.Client, options inboxOptions) ([]inboxEntry, []warning) {
	readStates := deduplicateChannelReadStates(client.ChannelReadStates())
	entries := make([]inboxEntry, 0, len(readStates))
	warnings := make([]warning, 0)
	unresolvedCount := 0

	for _, readState := range readStates {
		mentioned := readState.MentionCount > 0
		if !mentioned && !options.IncludeAll {
			continue
		}

		entry := newInboxEntry(readState)
		channel, channelErr := client.State.Channel(readState.ChannelID)
		if channelErr == nil {
			entry.ChannelAvailable = true
			view := newChannelView(*channel)
			entry.Channel = &view
			entry.GuildID = view.GuildID
			if channel.LastMessageID.IsValid() {
				latest := channel.LastMessageID.String()
				entry.LatestMessage = &latest
			}
			entry.Muted = channelMuted(client, *channel)
		} else if mentioned || readState.BadgeCount > 0 {
			unresolvedCount++
			issue := classifyInboxChannelError(channelErr)
			entry.Error = &issue
		}

		ordinaryUnread := isOrdinaryUnread(readState, channel)
		entry.Unread = mentioned || ordinaryUnread
		if !mentioned {
			if !ordinaryUnread || (entry.Muted && !options.IncludeMuted) {
				continue
			}
			entry.AttentionCause = "unread"
		}

		if options.IncludeMessages {
			if channelErr != nil {
				context := unavailableUnreadContext(readState)
				entry.Context = &context
			} else {
				context, contextWarning := fetchUnreadContext(client, readState, options.LimitPerChannel)
				entry.Context = &context
				if contextWarning != nil {
					warnings = append(warnings, *contextWarning)
				}
			}
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].MentionCount != entries[j].MentionCount {
			return entries[i].MentionCount > entries[j].MentionCount
		}
		if entries[i].LatestMessage != nil && entries[j].LatestMessage != nil && *entries[i].LatestMessage != *entries[j].LatestMessage {
			return *entries[i].LatestMessage > *entries[j].LatestMessage
		}
		return entries[i].ChannelID < entries[j].ChannelID
	})
	if unresolvedCount > 0 {
		warnings = append(warnings, warning{
			Code:    "channels_unavailable",
			Message: "Unread read-state entries for unavailable channels were retained by channel ID; message context was not requested for them",
			Count:   unresolvedCount,
		})
	}
	return entries, warnings
}

func newInboxEntry(readState gateway.ReadState) inboxEntry {
	entry := inboxEntry{
		ChannelID:      readState.ChannelID.String(),
		MentionCount:   readState.MentionCount,
		BadgeCount:     readState.BadgeCount,
		Unread:         readState.MentionCount > 0,
		AttentionCause: "mention",
	}
	if readState.LastMessageID.IsValid() {
		cursor := readState.LastMessageID.String()
		entry.ReadCursor = &cursor
	}
	return entry
}

func isOrdinaryUnread(readState gateway.ReadState, channel *discord.Channel) bool {
	if channel != nil && channel.LastMessageID.IsValid() && readState.LastMessageID.IsValid() {
		return channel.LastMessageID > readState.LastMessageID
	}
	return readState.BadgeCount > 0
}

func channelMuted(client *discordclient.Client, channel discord.Channel) bool {
	if client.ChannelIsMuted(channel) {
		return true
	}
	switch channel.Type {
	case discord.GuildAnnouncementThread, discord.GuildPublicThread, discord.GuildPrivateThread:
		return !client.State.ThreadState.ThreadIsJoined(channel.ID)
	default:
		return false
	}
}

func fetchUnreadContext(client *discordclient.Client, readState gateway.ReadState, limit int) (unreadContext, *warning) {
	context := unreadContext{
		Kind:                    "unread_span",
		ExpectedMentionCount:    readState.MentionCount,
		ExactMentionsIdentified: false,
		Available:               true,
		Messages:                []messageView{},
	}

	var (
		messages []discord.Message
		hasMore  bool
		err      error
	)
	if readState.LastMessageID.IsValid() {
		messages, hasMore, err = fetchAfterAscending(client, readState.ChannelID, readState.LastMessageID, limit)
	} else {
		messages, hasMore, _, err = fetchMessagePage(client, readState.ChannelID, 0, 0, 0, limit)
	}
	if err != nil {
		context.Truncated = true
		return context, &warning{
			Code:      "message_context_failed",
			Message:   err.Error(),
			ChannelID: readState.ChannelID.String(),
		}
	}
	context.Truncated = hasMore || !readState.LastMessageID.IsValid()
	context.Complete = !context.Truncated
	context.Messages = make([]messageView, len(messages))
	for i, message := range messages {
		context.Messages[i] = newMessageView(message)
	}
	return context, nil
}

func unavailableUnreadContext(readState gateway.ReadState) unreadContext {
	return unreadContext{
		Kind:                    "unread_span",
		ExpectedMentionCount:    readState.MentionCount,
		ExactMentionsIdentified: false,
		Available:               false,
		Complete:                false,
		Truncated:               true,
		Messages:                []messageView{},
	}
}

func classifyInboxChannelError(err error) inboxError {
	issue := inboxError{Code: "channel_unresolved", Message: "Channel metadata is unavailable"}
	var httpErr httputil.HTTPError
	if !errors.As(err, &httpErr) {
		return issue
	}
	switch httpErr.Status {
	case 403:
		issue.Code = "missing_access"
		issue.Message = "The authenticated profile no longer has access to this channel"
	case 404:
		issue.Code = "unknown_channel"
		issue.Message = "The channel was deleted or is no longer visible to the authenticated profile"
	}
	return issue
}

func deduplicateChannelReadStates(states []gateway.ReadState) []gateway.ReadState {
	byChannel := make(map[discord.ChannelID]gateway.ReadState, len(states))
	for _, state := range states {
		if state.Type != gateway.ReadStateTypeChannel || !state.ChannelID.IsValid() {
			continue
		}
		byChannel[state.ChannelID] = state
	}
	result := make([]gateway.ReadState, 0, len(byChannel))
	for _, state := range byChannel {
		result = append(result, state)
	}
	return result
}

func printInboxUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: discoctl inbox unread [--all] [--include-muted]
      [--messages] [--limit-per-channel 20] [--json]

The default is mentions-only across the account. Mentions remain visible when muted.
--all adds ordinary unread channels; --include-muted applies only to those ordinary
unreads. Message context never acknowledges or marks a channel read.`)
}

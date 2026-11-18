package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/utils/json/option"
	"github.com/grievouz/discoctl/internal/discordclient"
)

type messageMutationPlan struct {
	Action          string         `json:"action"`
	Applied         bool           `json:"applied"`
	DryRun          bool           `json:"dry_run"`
	Message         messageRefView `json:"message"`
	Content         *string        `json:"content,omitempty"`
	AllowedMentions *[]string      `json:"allowed_mentions,omitempty"`
	UnreadGuard     string         `json:"unread_guard"`
}

func runMessageMutation(ctx context.Context, action string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		printMessageMutationUsage(stdout, action)
		return nil
	}
	reference, args, err := takeLeadingReference(args)
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("messages "+action, flag.ContinueOnError)
	flags.SetOutput(stderr)
	channel := flags.String("channel", "", "channel ID")
	message := flags.String("message", "", "message ID")
	dryRun := flags.Bool("dry-run", false, "validate locally without connecting to Discord")
	ignoreUnread := flags.Bool("ignore-unread", false, "bypass the unread guard explicitly")
	var yes bool
	options := messageWriteOptions{NoPingReply: true}
	if action == "edit" {
		flags.StringVar(&options.Text, "text", "", "replacement message text")
		flags.StringVar(&options.MessageAlias, "msg", "", "alias for --text")
		flags.BoolVar(&options.ReadStdin, "stdin", false, "read replacement text from standard input")
		flags.BoolVar(&options.NoMentions, "no-mentions", false, "disable all mention parsing")
		flags.BoolVar(&options.NoUserMentions, "no-user-mentions", false, "disable user mention parsing")
		flags.BoolVar(&options.NoRoleMentions, "no-role-mentions", false, "disable role mention parsing")
		flags.BoolVar(&options.NoEveryone, "no-everyone", false, "disable @everyone and @here parsing")
	} else {
		flags.BoolVar(&yes, "yes", false, "confirm deletion of this single message")
	}
	output := addJSONFlag(flags)
	if err := parseFlags(flags, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printMessageMutationUsage(stdout, action)
			return nil
		}
		return err
	}
	if flags.NArg() > 1 || (flags.NArg() == 1 && reference != "") {
		return invalidArgumentsf("messages %s accepts at most one message reference", action)
	}
	if flags.NArg() == 1 {
		reference = flags.Arg(0)
	}
	if reference == "" && (*channel == "" || *message == "") {
		return invalidArgumentsf("messages %s requires a message reference or --channel and --message", action)
	}
	// Reuse the strict, mutually exclusive reference forms used by reactions.
	ref, err := resolveReactionMessage(reference, *channel, *message)
	if err != nil {
		return err
	}
	plan := messageMutationPlan{
		Action: action + "_message", DryRun: *dryRun, Message: ref.view(),
		UnreadGuard: "enforced_at_" + action,
	}
	if *ignoreUnread {
		plan.UnreadGuard = "bypassed"
	}
	var data api.EditMessageData
	if action == "edit" {
		content, err := messageContent(options, stdin)
		if err != nil {
			return err
		}
		plan.Content = &content
		data.Content = option.NewNullableString(content)
		mentions, names := makeAllowedMentions(options)
		data.AllowedMentions = mentions
		plan.AllowedMentions = &names
	} else if !*dryRun && !yes {
		return invalidArguments(errors.New("messages delete requires --yes; use --dry-run to preview without deleting"))
	}
	if *dryRun {
		return output.writeJSON(stdout, plan, nil, nil)
	}
	return withDiscordClient(ctx, func(client *discordclient.Client) error {
		return applyMessageMutation(client.State.Session.Client, ref, plan, data, func() error {
			status, err := client.ChannelReadStatus(ref.ChannelID)
			if err != nil {
				return fmt.Errorf("check unread guard: %w; use --ignore-unread to proceed anyway", err)
			}
			return checkUnreadGuard(ref.ChannelID, status)
		}, output, stdout)
	})
}

// Discord is authoritative for authorship, channel access and moderation rights.
// Do not infer these permissions from potentially stale gateway/cache state.
func applyMessageMutation(client *api.Client, ref messageRef, plan messageMutationPlan, data api.EditMessageData, guard func() error, output *outputOptions, stdout io.Writer) error {
	if plan.UnreadGuard != "bypassed" {
		if err := guard(); err != nil {
			return err
		}
	}
	if plan.Action == "edit_message" {
		message, err := client.EditMessageComplex(ref.ChannelID, ref.MessageID, data)
		if err != nil {
			return fmt.Errorf("edit message (only the original author can edit content): %w", err)
		}
		return output.writeJSON(stdout, newMessageView(*message), nil, nil)
	}
	if err := client.DeleteMessage(ref.ChannelID, ref.MessageID, ""); err != nil {
		return fmt.Errorf("delete message (deleting another author's guild message requires MANAGE_MESSAGES): %w", err)
	}
	plan.Applied = true
	return output.writeJSON(stdout, plan, nil, nil)
}

func printMessageMutationUsage(w io.Writer, action string) {
	fmt.Fprintf(w, `Usage:
  discoctl messages %s --channel <channel-id> --message <message-id>
      [--ignore-unread] [--dry-run] [--pretty] [--json]

A Discord message URL or channelID/messageID may be supplied instead of both IDs.
The unread guard is enforced by default. Dry runs do not check remote permissions.
`, action)
	if action == "edit" {
		fmt.Fprintln(w, `Edit requires exactly one of --text <replacement>, --msg <replacement>, or --stdin.
Only your own message content can be edited. Empty content is rejected.
Attachments, embeds, components and flags are not modified.
Mentions are parsed by default, independently of the original message's settings.
Use --no-mentions, --no-user-mentions, --no-role-mentions or --no-everyone to suppress them.`)
	} else {
		fmt.Fprintln(w, `Delete requires --yes unless --dry-run is used. Deletion cannot be undone.
Only one message is deleted. Deleting another author's guild message requires
MANAGE_MESSAGES; this does not allow deleting other people's DM messages.`)
	}
}

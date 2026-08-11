package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/gateway"
	"github.com/grievouz/discoctl/internal/discordclient"
)

const maxCustomStatusLength = 128

type statusView struct {
	Presence string            `json:"presence"`
	Custom   *customStatusView `json:"custom,omitempty"`
}

type customStatusView struct {
	Text      string  `json:"text"`
	EmojiName string  `json:"emoji_name,omitempty"`
	EmojiID   *string `json:"emoji_id,omitempty"`
	ExpiresAt *string `json:"expires_at,omitempty"`
}

type statusPlan struct {
	Action   string            `json:"action"`
	DryRun   bool              `json:"dry_run"`
	Presence string            `json:"presence"`
	Custom   *customStatusView `json:"custom"`
}

func runStatus(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printStatusUsage(stdout)
		return nil
	}
	switch args[0] {
	case "show":
		return runStatusShow(ctx, args[1:], stdout, stderr)
	case "set":
		return runStatusSet(ctx, args[1:], stdout, stderr)
	case "clear":
		return runStatusClear(ctx, args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown status command %q; run 'discoctl status help'", args[0])
	}
}

func runStatusShow(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("status show", flag.ContinueOnError)
	flags.SetOutput(stderr)
	addJSONFlag(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := requireNoPositionals(flags); err != nil {
		return fmt.Errorf("status show: %w", err)
	}
	return withDiscordClient(ctx, func(client *discordclient.Client) error {
		presence, custom := currentStatus(client)
		return writeJSON(stdout, newStatusView(presence, custom), nil, nil)
	})
}

func runStatusSet(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if hasHelpArg(args) {
		printStatusUsage(stdout)
		return nil
	}
	flags := flag.NewFlagSet("status set", flag.ContinueOnError)
	flags.SetOutput(stderr)
	presenceValue := flags.String("presence", "", "online, idle, dnd, or invisible")
	textValue := flags.String("text", "", "custom status text")
	emojiValue := flags.String("emoji", "", "custom status Unicode emoji or name:emoji-id")
	dryRun := flags.Bool("dry-run", false, "validate and print the update without connecting to Discord")
	addJSONFlag(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := requireNoPositionals(flags); err != nil {
		return fmt.Errorf("status set: %w", err)
	}
	if *presenceValue == "" && *textValue == "" && *emojiValue == "" {
		return errors.New("status set requires --presence, --text, or --emoji")
	}
	if utf16Length(*textValue) > maxCustomStatusLength {
		return fmt.Errorf("custom status exceeds %d characters", maxCustomStatusLength)
	}
	presence, err := parsePresence(*presenceValue)
	if err != nil {
		return err
	}
	custom, err := makeCustomStatus(*textValue, *emojiValue)
	if err != nil {
		return err
	}
	if *textValue == "" && *emojiValue == "" {
		custom = nil
	}

	planPresence := string(presence)
	if presence == "" {
		planPresence = "preserve_current"
	}
	plan := statusPlan{
		Action:   "set_status",
		DryRun:   *dryRun,
		Presence: planPresence,
		Custom:   customStatusViewFromGateway(custom),
	}
	if *dryRun {
		return writeJSON(stdout, plan, nil, nil)
	}

	return withDiscordClient(ctx, func(client *discordclient.Client) error {
		if presence == "" {
			presence, _ = currentStatus(client)
		}
		if err := client.State.SetStatus(presence, custom); err != nil {
			return fmt.Errorf("set status: %w", err)
		}
		return writeJSON(stdout, newStatusView(presence, custom), nil, nil)
	})
}

func runStatusClear(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("status clear", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dryRun := flags.Bool("dry-run", false, "print the update without connecting to Discord")
	addJSONFlag(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := requireNoPositionals(flags); err != nil {
		return fmt.Errorf("status clear: %w", err)
	}
	plan := statusPlan{Action: "clear_custom_status", DryRun: *dryRun, Presence: "preserve_current"}
	if *dryRun {
		return writeJSON(stdout, plan, nil, nil)
	}
	return withDiscordClient(ctx, func(client *discordclient.Client) error {
		presence, _ := currentStatus(client)
		custom := &gateway.CustomUserStatus{}
		if err := client.State.SetStatus(presence, custom); err != nil {
			return fmt.Errorf("clear custom status: %w", err)
		}
		return writeJSON(stdout, newStatusView(presence, nil), nil, nil)
	})
}

func parsePresence(value string) (discord.Status, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "online":
		return discord.OnlineStatus, nil
	case "idle":
		return discord.IdleStatus, nil
	case "dnd", "do-not-disturb":
		return discord.DoNotDisturbStatus, nil
	case "invisible", "offline":
		return discord.InvisibleStatus, nil
	default:
		return "", fmt.Errorf("invalid presence %q; use online, idle, dnd, or invisible", value)
	}
}

func makeCustomStatus(text, emojiValue string) (*gateway.CustomUserStatus, error) {
	custom := &gateway.CustomUserStatus{Text: text}
	if emojiValue == "" {
		return custom, nil
	}
	emoji, err := parseReactionEmoji(emojiValue)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(emoji), ":", 2)
	custom.EmojiName = parts[0]
	if len(parts) == 2 {
		snowflake, err := discord.ParseSnowflake(parts[1])
		if err != nil {
			return nil, err
		}
		custom.EmojiID = discord.EmojiID(snowflake)
	}
	return custom, nil
}

func currentStatus(client *discordclient.Client) (discord.Status, *gateway.CustomUserStatus) {
	ready := client.State.Ready()
	if ready.UserSettings != nil {
		presence := ready.UserSettings.Status
		if presence == "" {
			presence = discord.OnlineStatus
		}
		return presence, ready.UserSettings.CustomStatus
	}
	if me, err := client.State.Me(); err == nil {
		if presence, err := client.State.PresenceStore.Presence(0, me.ID); err == nil && presence != nil && presence.Status != "" {
			return presence.Status, customStatusFromActivities(presence.Activities)
		}
	}
	return discord.OnlineStatus, nil
}

func customStatusFromActivities(activities []discord.Activity) *gateway.CustomUserStatus {
	for _, activity := range activities {
		if activity.Type != discord.CustomActivity {
			continue
		}
		custom := &gateway.CustomUserStatus{Text: activity.State}
		if activity.Emoji != nil {
			custom.EmojiID = activity.Emoji.ID
			custom.EmojiName = activity.Emoji.Name
		}
		return custom
	}
	return nil
}

func newStatusView(presence discord.Status, custom *gateway.CustomUserStatus) statusView {
	return statusView{Presence: string(presence), Custom: customStatusViewFromGateway(custom)}
}

func customStatusViewFromGateway(custom *gateway.CustomUserStatus) *customStatusView {
	if custom == nil || (custom.Text == "" && custom.EmojiName == "" && !custom.EmojiID.IsValid()) {
		return nil
	}
	view := &customStatusView{Text: custom.Text, EmojiName: custom.EmojiName}
	if custom.EmojiID.IsValid() {
		id := custom.EmojiID.String()
		view.EmojiID = &id
	}
	if custom.ExpiresAt.IsValid() {
		expires := custom.ExpiresAt.Time().UTC().Format(time.RFC3339)
		view.ExpiresAt = &expires
	}
	return view
}

func printStatusUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  discoctl status show [--json]
  discoctl status set [--presence online|idle|dnd|invisible]
      [--text <custom-status>] [--emoji <unicode-or-name:id>]
      [--dry-run] [--json]
  discoctl status clear [--dry-run] [--json]

If --presence is omitted while setting a custom status, the current presence is
preserved. Clearing removes the custom status and also preserves presence.`)
}

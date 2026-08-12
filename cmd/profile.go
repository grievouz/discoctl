package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/grievouz/discoctl/internal/discordclient"
)

const (
	maxProfileBioLength  = 190
	maxProfileImageBytes = 10 << 20
)

type profileView struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	Bio         string  `json:"bio"`
	Pronouns    string  `json:"pronouns"`
	AccentColor *string `json:"accent_color"`
	AvatarURL   string  `json:"avatar_url"`
	BannerURL   string  `json:"banner_url"`
}

type profileUpdatePlan struct {
	Action string            `json:"action"`
	DryRun bool              `json:"dry_run"`
	Fields []string          `json:"fields"`
	Bio    *string           `json:"bio,omitempty"`
	Color  *string           `json:"accent_color,omitempty"`
	Avatar *profileImagePlan `json:"avatar,omitempty"`
	Banner *profileImagePlan `json:"banner,omitempty"`
}

type profileImagePlan struct {
	Operation   string `json:"operation"`
	Path        string `json:"path,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Bytes       int    `json:"bytes,omitempty"`
}

type profileUpdateResult struct {
	Action  string       `json:"action"`
	Applied []string     `json:"applied"`
	Profile *profileView `json:"profile,omitempty"`
}

func runProfile(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printProfileUsage(stdout)
		return nil
	}
	switch args[0] {
	case "show":
		return runProfileShow(ctx, args[1:], stdout, stderr)
	case "update":
		return runProfileUpdate(ctx, args[1:], stdout, stderr)
	default:
		return invalidArgumentsf("unknown profile command %q; run 'discoctl profile help'", args[0])
	}
}

func runProfileShow(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("profile show", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := addJSONFlag(flags)
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := requireNoPositionals(flags); err != nil {
		return fmt.Errorf("profile show: %w", err)
	}
	return withDiscordClient(ctx, func(client *discordclient.Client) error {
		user, err := client.State.Session.Me()
		if err != nil {
			return fmt.Errorf("get current profile: %w", err)
		}
		return output.writeJSON(stdout, newProfileView(*user), nil, nil)
	})
}

func runProfileUpdate(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if hasHelpArg(args) {
		printProfileUsage(stdout)
		return nil
	}
	flags := flag.NewFlagSet("profile update", flag.ContinueOnError)
	flags.SetOutput(stderr)
	bio := flags.String("bio", "", "new profile bio")
	clearBio := flags.Bool("clear-bio", false, "clear the profile bio")
	color := flags.String("color", "", "accent color as #RRGGBB or RRGGBB")
	clearColor := flags.Bool("clear-color", false, "clear the accent color")
	avatarPath := flags.String("avatar", "", "path to a PNG, JPEG, or GIF avatar")
	clearAvatar := flags.Bool("clear-avatar", false, "remove the custom avatar")
	bannerPath := flags.String("banner", "", "path to a PNG, JPEG, or GIF banner")
	clearBanner := flags.Bool("clear-banner", false, "remove the profile banner")
	dryRun := flags.Bool("dry-run", false, "validate and print the update without connecting to Discord")
	output := addJSONFlag(flags)
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if err := requireNoPositionals(flags); err != nil {
		return fmt.Errorf("profile update: %w", err)
	}
	if (*bio != "" && *clearBio) || (*color != "" && *clearColor) || (*avatarPath != "" && *clearAvatar) || (*bannerPath != "" && *clearBanner) {
		return invalidArguments(errors.New("a profile field cannot be set and cleared in the same update"))
	}
	if utf16Length(*bio) > maxProfileBioLength {
		return invalidArgumentsf("profile bio exceeds %d characters", maxProfileBioLength)
	}

	profileFields := make(map[string]any)
	accountFields := make(map[string]any)
	plan := profileUpdatePlan{Action: "update_profile", DryRun: *dryRun, Fields: []string{}}
	if *bio != "" || *clearBio {
		value := *bio
		profileFields["bio"] = value
		plan.Bio = &value
		plan.Fields = append(plan.Fields, "bio")
	}
	if *color != "" || *clearColor {
		if *clearColor {
			profileFields["accent_color"] = nil
			value := ""
			plan.Color = &value
		} else {
			parsed, normalized, err := parseProfileColor(*color)
			if err != nil {
				return err
			}
			profileFields["accent_color"] = parsed
			plan.Color = &normalized
		}
		plan.Fields = append(plan.Fields, "accent_color")
	}
	if *avatarPath != "" || *clearAvatar {
		image, imagePlan, err := prepareProfileImage(*avatarPath, *clearAvatar)
		if err != nil {
			return invalidArgumentsf("avatar: %w", err)
		}
		accountFields["avatar"] = image
		plan.Avatar = &imagePlan
		plan.Fields = append(plan.Fields, "avatar")
	}
	if *bannerPath != "" || *clearBanner {
		image, imagePlan, err := prepareProfileImage(*bannerPath, *clearBanner)
		if err != nil {
			return invalidArgumentsf("banner: %w", err)
		}
		accountFields["banner"] = image
		plan.Banner = &imagePlan
		plan.Fields = append(plan.Fields, "banner")
	}
	if len(plan.Fields) == 0 {
		return invalidArguments(errors.New("profile update requires at least one field"))
	}
	if *dryRun {
		return output.writeJSON(stdout, plan, nil, nil)
	}

	return withDiscordClient(ctx, func(client *discordclient.Client) error {
		applied := make([]string, 0, len(plan.Fields))
		if len(accountFields) > 0 {
			if _, err := client.PatchCurrentUser(accountFields); err != nil {
				return fmt.Errorf("update avatar or banner: %w", err)
			}
			if _, ok := accountFields["avatar"]; ok {
				applied = append(applied, "avatar")
			}
			if _, ok := accountFields["banner"]; ok {
				applied = append(applied, "banner")
			}
		}
		if len(profileFields) > 0 {
			if err := client.PatchUserProfile(profileFields); err != nil {
				return fmt.Errorf("update bio or accent color after applying %v: %w", applied, err)
			}
			if _, ok := profileFields["bio"]; ok {
				applied = append(applied, "bio")
			}
			if _, ok := profileFields["accent_color"]; ok {
				applied = append(applied, "accent_color")
			}
		}

		result := profileUpdateResult{Action: "update_profile", Applied: applied}
		if user, err := client.State.Session.Me(); err == nil {
			view := newProfileView(*user)
			result.Profile = &view
		}
		return output.writeJSON(stdout, result, nil, nil)
	})
}

func newProfileView(user discord.User) profileView {
	view := profileView{
		ID:          user.ID.String(),
		Username:    user.Username,
		DisplayName: user.DisplayOrUsername(),
		Bio:         user.Bio,
		Pronouns:    user.Pronouns,
		AvatarURL:   user.AvatarURL(),
		BannerURL:   user.BannerURL(),
	}
	if user.Accent != 0 {
		color := fmt.Sprintf("#%06X", uint32(user.Accent)&0xFFFFFF)
		view.AccentColor = &color
	}
	return view
}

func parseProfileColor(value string) (int, string, error) {
	normalized := strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(normalized) != 6 {
		return 0, "", invalidArguments(errors.New("profile color must contain exactly six hexadecimal digits"))
	}
	parsed, err := strconv.ParseUint(normalized, 16, 24)
	if err != nil {
		return 0, "", invalidArgumentsf("invalid profile color %q: %w", value, err)
	}
	return int(parsed), "#" + strings.ToUpper(normalized), nil
}

func prepareProfileImage(path string, clear bool) (*api.Image, profileImagePlan, error) {
	if clear {
		return api.NullImage, profileImagePlan{Operation: "clear"}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, profileImagePlan{}, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxProfileImageBytes+1))
	if err != nil {
		return nil, profileImagePlan{}, err
	}
	if len(content) > maxProfileImageBytes {
		return nil, profileImagePlan{}, fmt.Errorf("image exceeds %d bytes", maxProfileImageBytes)
	}
	contentType := http.DetectContentType(content)
	image := &api.Image{ContentType: contentType, Content: content}
	if err := image.Validate(maxProfileImageBytes); err != nil {
		return nil, profileImagePlan{}, err
	}
	return image, profileImagePlan{
		Operation:   "set",
		Path:        path,
		ContentType: contentType,
		Bytes:       len(content),
	}, nil
}

func printProfileUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  discoctl profile show [--pretty] [--json]
  discoctl profile update [--bio <text> | --clear-bio]
      [--color <#RRGGBB> | --clear-color]
      [--avatar <file> | --clear-avatar]
      [--banner <file> | --clear-banner]
      [--dry-run] [--pretty] [--json]

Image files are limited to 10 MiB and must be PNG, JPEG, or GIF. A multi-field
profile update is not atomic; use --dry-run to validate it first.`)
}

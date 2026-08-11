package cmd

import (
	"strings"
	"time"

	"github.com/ayn2op/arikawa/v3/discord"
)

type guildView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type channelView struct {
	ID            string   `json:"id"`
	GuildID       *string  `json:"guild_id"`
	ParentID      *string  `json:"parent_id"`
	Type          string   `json:"type"`
	Name          string   `json:"name"`
	Topic         string   `json:"topic,omitempty"`
	Recipients    []string `json:"recipients,omitempty"`
	LastMessageID *string  `json:"last_message_id"`
}

type userView struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Bot         bool   `json:"bot"`
}

type attachmentView struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	Description string `json:"description,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Size        uint64 `json:"size"`
	URL         string `json:"url"`
}

type reactionView struct {
	EmojiID   *string `json:"emoji_id"`
	EmojiName string  `json:"emoji_name"`
	Count     int     `json:"count"`
	Me        bool    `json:"me"`
}

type messageReferenceView struct {
	GuildID   *string `json:"guild_id"`
	ChannelID *string `json:"channel_id"`
	MessageID *string `json:"message_id"`
}

type messageView struct {
	ID              string                `json:"id"`
	GuildID         *string               `json:"guild_id"`
	ChannelID       string                `json:"channel_id"`
	URL             string                `json:"url"`
	Type            uint                  `json:"type"`
	Author          userView              `json:"author"`
	Content         string                `json:"content"`
	Timestamp       string                `json:"timestamp"`
	EditedTimestamp *string               `json:"edited_timestamp"`
	MentionEveryone bool                  `json:"mention_everyone"`
	MentionUserIDs  []string              `json:"mention_user_ids"`
	MentionRoleIDs  []string              `json:"mention_role_ids"`
	Attachments     []attachmentView      `json:"attachments"`
	Embeds          []discord.Embed       `json:"embeds"`
	Reactions       []reactionView        `json:"reactions"`
	Reference       *messageReferenceView `json:"reference,omitempty"`
}

func newGuildView(guild discord.Guild) guildView {
	return guildView{ID: guild.ID.String(), Name: guild.Name}
}

func newChannelView(channel discord.Channel) channelView {
	view := channelView{
		ID:         channel.ID.String(),
		Type:       channelTypeName(channel.Type),
		Name:       channelDisplayName(channel),
		Topic:      channel.Topic,
		Recipients: make([]string, 0, len(channel.DMRecipients)),
	}
	if channel.GuildID.IsValid() {
		id := channel.GuildID.String()
		view.GuildID = &id
	}
	if channel.ParentID.IsValid() {
		id := channel.ParentID.String()
		view.ParentID = &id
	}
	if channel.LastMessageID.IsValid() {
		id := channel.LastMessageID.String()
		view.LastMessageID = &id
	}
	for _, recipient := range channel.DMRecipients {
		view.Recipients = append(view.Recipients, recipient.DisplayOrUsername())
	}
	return view
}

func newMessageView(message discord.Message) messageView {
	view := messageView{
		ID:              message.ID.String(),
		ChannelID:       message.ChannelID.String(),
		URL:             message.URL(),
		Type:            uint(message.Type),
		Author:          newUserView(message.Author),
		Content:         message.Content,
		Timestamp:       message.Timestamp.Time().UTC().Format(time.RFC3339Nano),
		MentionEveryone: message.MentionEveryone,
		MentionUserIDs:  make([]string, 0, len(message.Mentions)),
		MentionRoleIDs:  make([]string, 0, len(message.MentionRoleIDs)),
		Attachments:     make([]attachmentView, 0, len(message.Attachments)),
		Embeds:          message.Embeds,
		Reactions:       make([]reactionView, 0, len(message.Reactions)),
	}
	if message.GuildID.IsValid() {
		id := message.GuildID.String()
		view.GuildID = &id
	}
	if message.EditedTimestamp.IsValid() {
		timestamp := message.EditedTimestamp.Time().UTC().Format(time.RFC3339Nano)
		view.EditedTimestamp = &timestamp
	}
	for _, mention := range message.Mentions {
		view.MentionUserIDs = append(view.MentionUserIDs, mention.ID.String())
	}
	for _, roleID := range message.MentionRoleIDs {
		view.MentionRoleIDs = append(view.MentionRoleIDs, roleID.String())
	}
	for _, attachment := range message.Attachments {
		view.Attachments = append(view.Attachments, attachmentView{
			ID:          attachment.ID.String(),
			Filename:    attachment.Filename,
			Description: attachment.Description,
			ContentType: attachment.ContentType,
			Size:        attachment.Size,
			URL:         string(attachment.URL),
		})
	}
	for _, reaction := range message.Reactions {
		view.Reactions = append(view.Reactions, newReactionView(reaction))
	}
	if reference := message.Reference; reference != nil {
		view.Reference = &messageReferenceView{}
		if reference.GuildID.IsValid() {
			id := reference.GuildID.String()
			view.Reference.GuildID = &id
		}
		if reference.ChannelID.IsValid() {
			id := reference.ChannelID.String()
			view.Reference.ChannelID = &id
		}
		if reference.MessageID.IsValid() {
			id := reference.MessageID.String()
			view.Reference.MessageID = &id
		}
	}
	return view
}

func newUserView(user discord.User) userView {
	return userView{
		ID:          user.ID.String(),
		Username:    user.Username,
		DisplayName: user.DisplayOrUsername(),
		Bot:         user.Bot,
	}
}

func newReactionView(reaction discord.Reaction) reactionView {
	view := reactionView{
		EmojiName: reaction.Emoji.Name,
		Count:     reaction.Count,
		Me:        reaction.Me,
	}
	if reaction.Emoji.ID.IsValid() {
		id := reaction.Emoji.ID.String()
		view.EmojiID = &id
	}
	return view
}

func channelDisplayName(channel discord.Channel) string {
	if channel.Name != "" {
		return channel.Name
	}
	names := make([]string, 0, len(channel.DMRecipients))
	for _, recipient := range channel.DMRecipients {
		names = append(names, recipient.DisplayOrUsername())
	}
	if len(names) > 0 {
		return strings.Join(names, ", ")
	}
	return channel.ID.String()
}

func channelTypeName(channelType discord.ChannelType) string {
	switch channelType {
	case discord.GuildText:
		return "guild_text"
	case discord.DirectMessage:
		return "direct_message"
	case discord.GuildVoice:
		return "guild_voice"
	case discord.GroupDM:
		return "group_direct_message"
	case discord.GuildCategory:
		return "guild_category"
	case discord.GuildAnnouncement:
		return "guild_announcement"
	case discord.GuildStore:
		return "guild_store"
	case discord.GuildAnnouncementThread:
		return "guild_announcement_thread"
	case discord.GuildPublicThread:
		return "guild_public_thread"
	case discord.GuildPrivateThread:
		return "guild_private_thread"
	case discord.GuildStageVoice:
		return "guild_stage_voice"
	case discord.GuildDirectory:
		return "guild_directory"
	case discord.GuildForum:
		return "guild_forum"
	default:
		return "unknown"
	}
}

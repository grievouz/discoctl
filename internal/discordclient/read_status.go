package discordclient

import (
	"fmt"

	"github.com/ayn2op/arikawa/v3/discord"
)

type ChannelReadStatus struct {
	LatestMessageID discord.MessageID
	ReadCursor      discord.MessageID
	Verifiable      bool
	Unread          bool
}

func (c *Client) ChannelReadStatus(channelID discord.ChannelID) (ChannelReadStatus, error) {
	channel, err := c.State.Session.Channel(channelID)
	if err != nil {
		return ChannelReadStatus{}, fmt.Errorf("get latest channel state: %w", err)
	}

	readCursor := discord.MessageID(0)
	if readState := c.State.ReadState.ReadState(channelID); readState != nil {
		readCursor = readState.LastMessageID
	}
	return evaluateChannelReadStatus(channel.LastMessageID, readCursor), nil
}

func evaluateChannelReadStatus(latestMessageID, readCursor discord.MessageID) ChannelReadStatus {
	status := ChannelReadStatus{
		LatestMessageID: latestMessageID,
		ReadCursor:      readCursor,
	}
	if !latestMessageID.IsValid() {
		status.Verifiable = true
		return status
	}
	if !readCursor.IsValid() {
		return status
	}
	status.Verifiable = true
	status.Unread = readCursor < latestMessageID
	return status
}

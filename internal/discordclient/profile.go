package discordclient

import (
	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/discord"
	"github.com/ayn2op/arikawa/v3/utils/httputil"
)

func (c *Client) PatchUserProfile(fields map[string]any) error {
	return c.State.Session.FastRequest(
		"PATCH",
		api.EndpointMe+"/profile",
		httputil.WithJSONBody(fields),
	)
}

func (c *Client) PatchCurrentUser(fields map[string]any) (*discord.User, error) {
	var user *discord.User
	err := c.State.Session.RequestJSON(
		&user,
		"PATCH",
		api.EndpointMe,
		httputil.WithJSONBody(fields),
	)
	return user, err
}

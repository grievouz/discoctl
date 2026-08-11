package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/grievouz/discoctl/internal/auth"
	"github.com/grievouz/discoctl/internal/discordclient"
)

const commandTimeout = 45 * time.Second

func withDiscordClient(ctx context.Context, fn func(*discordclient.Client) error) (err error) {
	token, _, err := auth.LoadToken()
	if err != nil {
		if errors.Is(err, auth.ErrNoToken) {
			return errors.New("not authenticated; run 'discoctl auth login'")
		}
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	client := discordclient.New(token)
	if err := client.Open(ctx); err != nil {
		return err
	}
	defer func() {
		if closeErr := client.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	if err := fn(client); err != nil {
		return fmt.Errorf("Discord command: %w", err)
	}
	return nil
}

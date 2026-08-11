package auth

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/grievouz/discoctl/internal/keyring"
)

const EnvToken = "DISCOCTL_TOKEN"

var ErrNoToken = errors.New("no Discord token is configured")

type TokenSource string

const (
	TokenSourceEnvironment TokenSource = EnvToken
	TokenSourceKeyring     TokenSource = "system keyring"
)

func LoadToken() (string, TokenSource, error) {
	if token := strings.TrimSpace(os.Getenv(EnvToken)); token != "" {
		return token, TokenSourceEnvironment, nil
	}

	token, err := keyring.GetToken()
	if err != nil {
		if keyring.IsNotFound(err) {
			return "", "", ErrNoToken
		}
		return "", "", fmt.Errorf("load token from system keyring: %w", err)
	}
	if token == "" {
		return "", "", ErrNoToken
	}
	return token, TokenSourceKeyring, nil
}

func SaveToken(token string) error {
	if token = strings.TrimSpace(token); token == "" {
		return errors.New("cannot store an empty token")
	}
	if err := keyring.SetToken(token); err != nil {
		return fmt.Errorf("store token in system keyring: %w", err)
	}
	return nil
}

func DeleteToken() error {
	if err := keyring.DeleteToken(); err != nil && !keyring.IsNotFound(err) {
		return fmt.Errorf("delete token from system keyring: %w", err)
	}
	return nil
}

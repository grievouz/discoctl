package keyring

import (
	"errors"

	"github.com/grievouz/discoctl/internal/consts"
	"github.com/zalando/go-keyring"
)

const (
	keyringService = consts.Name
	keyringUser    = "token"
)

type store interface {
	Get(service, user string) (string, error)
	Set(service, user, secret string) error
	Delete(service, user string) error
}

type systemKeyring struct{}

func (systemKeyring) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (systemKeyring) Set(service, user, secret string) error {
	return keyring.Set(service, user, secret)
}

func (systemKeyring) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

func GetToken() (string, error) {
	return tokenStore.Get(keyringService, keyringUser)
}

func SetToken(s string) error {
	return tokenStore.Set(keyringService, keyringUser, s)
}

func DeleteToken() error {
	return tokenStore.Delete(keyringService, keyringUser)
}

func IsNotFound(err error) bool {
	return errors.Is(err, keyring.ErrNotFound)
}

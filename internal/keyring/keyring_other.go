//go:build !darwin

package keyring

var tokenStore store = systemKeyring{}

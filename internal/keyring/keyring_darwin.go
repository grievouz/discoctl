//go:build darwin

package keyring

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

const securityPath = "/usr/bin/security"

// The same encoding go-keyring uses, so tokens it stored stay readable.
const (
	base64Prefix = "go-keyring-base64:"
	hexPrefix    = "go-keyring-encoded:"
)

// Outside a GUI login (for example over SSH) the default keychain resolves to
// the System keychain, which users cannot write to, so name the login
// keychain explicitly.
var tokenStore store = loginKeychain{}

type loginKeychain struct{}

func (loginKeychain) Get(service, user string) (string, error) {
	path, err := loginKeychainPath()
	if err != nil {
		return "", err
	}
	out, err := runSecurity("", "find-generic-password", "-s", service, "-a", user, "-w", path)
	if err != nil {
		return "", err
	}
	return decodeSecret(strings.TrimSpace(out))
}

func (loginKeychain) Set(service, user, secret string) error {
	path, err := loginKeychainPath()
	if err != nil {
		return err
	}
	encoded := base64Prefix + base64.StdEncoding.EncodeToString([]byte(secret))
	// Interactive mode reads the command from stdin, keeping the secret out of
	// the process arguments.
	command := strings.Join([]string{
		"add-generic-password", "-U",
		"-s", quote(service),
		"-a", quote(user),
		"-w", quote(encoded),
		quote(path),
	}, " ") + "\n"
	_, err = runSecurity(command, "-i")
	return err
}

func (loginKeychain) Delete(service, user string) error {
	path, err := loginKeychainPath()
	if err != nil {
		return err
	}
	_, err = runSecurity("", "delete-generic-password", "-s", service, "-a", user, path)
	return err
}

func loginKeychainPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate login keychain: %w", err)
	}
	return filepath.Join(home, "Library", "Keychains", "login.keychain-db"), nil
}

func runSecurity(stdin string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(securityPath, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if strings.Contains(message, "could not be found") {
			return "", keyring.ErrNotFound
		}
		if message == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, message)
	}
	return stdout.String(), nil
}

func decodeSecret(value string) (string, error) {
	if encoded, ok := strings.CutPrefix(value, base64Prefix); ok {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		return string(decoded), err
	}
	if encoded, ok := strings.CutPrefix(value, hexPrefix); ok {
		decoded, err := hex.DecodeString(encoded)
		return string(decoded), err
	}
	return value, nil
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

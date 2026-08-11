package cmd

import (
	"bytes"
	"context"
	"testing"
)

func TestAuthLoginRejectsMultipleTokenSources(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	err := RunContext(
		context.Background(),
		[]string{"auth", "login", "--token", "example-token", "--token-stdin"},
		bytes.NewBufferString("stdin-token"),
		&stdout,
		&stderr,
	)
	if err == nil {
		t.Fatal("expected conflicting token sources to fail")
	}
}

func TestAuthLoginRejectsLocalCaptchaWithToken(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	err := RunContext(
		context.Background(),
		[]string{"auth", "login", "--token", "example-token", "--local-captcha"},
		bytes.NewBuffer(nil),
		&stdout,
		&stderr,
	)
	if err == nil {
		t.Fatal("expected --local-captcha with token import to fail")
	}
}

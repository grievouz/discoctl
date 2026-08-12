package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/ayn2op/arikawa/v3/utils/httputil"
)

func TestWriteErrorPreservesStableSemanticCode(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("messages send: %w", preconditionFailed(
		"channel_unread",
		map[string]any{"channel_id": "123"},
		errors.New("channel 123 has unread messages"),
	))
	var stderr bytes.Buffer
	if got := WriteError(&stderr, err); got != exitPrecondition {
		t.Fatalf("exit code = %d, want %d", got, exitPrecondition)
	}

	var result errorEnvelope
	if err := json.Unmarshal(stderr.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error.Code != "channel_unread" || result.Error.Message != err.Error() {
		t.Fatalf("unexpected error: %#v", result.Error)
	}
	if result.Error.Details["channel_id"] != "123" {
		t.Fatalf("unexpected details: %#v", result.Error.Details)
	}
}

func TestClassifyDiscordHTTPError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		status    int
		wantCode  string
		wantExit  int
		retryable bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, wantCode: "authentication_failed", wantExit: exitAuthentication},
		{name: "forbidden", status: http.StatusForbidden, wantCode: "permission_denied", wantExit: exitPermissionDenied},
		{name: "not found", status: http.StatusNotFound, wantCode: "not_found", wantExit: exitNotFound},
		{name: "conflict", status: http.StatusConflict, wantCode: "conflict", wantExit: exitPrecondition},
		{name: "rate limited", status: http.StatusTooManyRequests, wantCode: "rate_limited", wantExit: exitTemporary, retryable: true},
		{name: "server error", status: http.StatusBadGateway, wantCode: "discord_unavailable", wantExit: exitTemporary, retryable: true},
		{name: "other", status: http.StatusBadRequest, wantCode: "discord_error", wantExit: exitUnexpected},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			view, exitCode := classifyError(fmt.Errorf("request: %w", httputil.HTTPError{
				Status:  test.status,
				Code:    10003,
				Message: "example",
			}))
			if view.Code != test.wantCode || exitCode != test.wantExit || view.Retryable != test.retryable {
				t.Fatalf("view = %#v, exit = %d", view, exitCode)
			}
			if view.Details["http_status"] != float64(test.status) && view.Details["http_status"] != test.status {
				t.Fatalf("details = %#v", view.Details)
			}
		})
	}
}

func TestClassifyUnknownError(t *testing.T) {
	t.Parallel()

	view, exitCode := classifyError(errors.New("boom"))
	if view.Code != "internal_error" || exitCode != exitUnexpected || view.Retryable {
		t.Fatalf("view = %#v, exit = %d", view, exitCode)
	}
}

func TestInvalidFlagProducesOnlyStructuredError(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	err := RunContext(
		context.Background(),
		[]string{"guilds", "list", "--bogus"},
		bytes.NewReader(nil),
		&stdout,
		&stderr,
	)
	if err == nil {
		t.Fatal("expected invalid flag to fail")
	}
	if stderr.Len() != 0 {
		t.Fatalf("flag package contaminated stderr: %q", stderr.String())
	}
	if exitCode := WriteError(&stderr, err); exitCode != exitInvalidArguments {
		t.Fatalf("exit code = %d, want %d", exitCode, exitInvalidArguments)
	}

	var result errorEnvelope
	if err := json.Unmarshal(stderr.Bytes(), &result); err != nil {
		t.Fatalf("decode structured error: %v\n%s", err, stderr.String())
	}
	if result.Error.Code != "invalid_arguments" {
		t.Fatalf("error code = %q", result.Error.Code)
	}
}

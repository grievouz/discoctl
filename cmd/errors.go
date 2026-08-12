package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/ayn2op/arikawa/v3/utils/httputil"
)

const (
	exitUnexpected       = 1
	exitInvalidArguments = 2
	exitAuthentication   = 3
	exitNotFound         = 4
	exitPermissionDenied = 5
	exitPrecondition     = 6
	exitTemporary        = 7
)

const (
	errorCodeInvalidArguments       = "invalid_arguments"
	errorCodeAuthenticationRequired = "authentication_required"
	errorCodeAuthenticationFailed   = "authentication_failed"
	errorCodeNotFound               = "not_found"
	errorCodePermissionDenied       = "permission_denied"
	errorCodeConflict               = "conflict"
	errorCodeRateLimited            = "rate_limited"
	errorCodeRequestTimeout         = "request_timeout"
	errorCodeRequestCanceled        = "request_canceled"
	errorCodeDiscordUnavailable     = "discord_unavailable"
	errorCodeInvalidDiscordResponse = "invalid_discord_response"
	errorCodeDiscord                = "discord_error"
	errorCodeInternal               = "internal_error"
	errorCodeChannelUnread          = "channel_unread"
	errorCodeReadStateUnverifiable  = "read_state_unverifiable"
	errorCodeChannelAdvanced        = "channel_advanced"
)

type structuredCommandError struct {
	code       string
	exitCode   int
	retryable  bool
	details    map[string]any
	underlying error
}

func (err *structuredCommandError) Error() string {
	return err.underlying.Error()
}

func (err *structuredCommandError) Unwrap() error {
	return err.underlying
}

type errorEnvelope struct {
	SchemaVersion string    `json:"schema_version"`
	Error         errorView `json:"error"`
}

type errorView struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details,omitempty"`
}

func invalidArguments(err error) error {
	return newStructuredCommandError(errorCodeInvalidArguments, exitInvalidArguments, false, nil, err)
}

func invalidArgumentsf(format string, args ...any) error {
	return invalidArguments(fmt.Errorf(format, args...))
}

func authenticationRequired(err error) error {
	return newStructuredCommandError(errorCodeAuthenticationRequired, exitAuthentication, false, nil, err)
}

func preconditionFailed(code string, details map[string]any, err error) error {
	return newStructuredCommandError(code, exitPrecondition, false, details, err)
}

func newStructuredCommandError(code string, exitCode int, retryable bool, details map[string]any, err error) error {
	if err == nil {
		err = errors.New(code)
	}
	return &structuredCommandError{
		code:       code,
		exitCode:   exitCode,
		retryable:  retryable,
		details:    details,
		underlying: err,
	}
}

// WriteError emits one compact JSON error to w and returns the process exit code.
func WriteError(w io.Writer, err error) int {
	view, exitCode := classifyError(err)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(errorEnvelope{SchemaVersion: schemaVersion, Error: view})
	return exitCode
}

func classifyError(err error) (errorView, int) {
	var structured *structuredCommandError
	if errors.As(err, &structured) {
		return errorView{
			Code:      structured.code,
			Message:   err.Error(),
			Retryable: structured.retryable,
			Details:   structured.details,
		}, structured.exitCode
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return errorView{Code: errorCodeRequestTimeout, Message: err.Error(), Retryable: true}, exitTemporary
	}
	if errors.Is(err, context.Canceled) {
		return errorView{Code: errorCodeRequestCanceled, Message: err.Error(), Retryable: false}, exitUnexpected
	}

	if httpErr, ok := discordHTTPErrorForOutput(err); ok {
		return classifyDiscordHTTPError(err, httpErr)
	}

	var requestErr httputil.RequestError
	if errors.As(err, &requestErr) {
		return errorView{Code: errorCodeDiscordUnavailable, Message: err.Error(), Retryable: true}, exitTemporary
	}
	var jsonErr httputil.JSONError
	if errors.As(err, &jsonErr) {
		return errorView{Code: errorCodeInvalidDiscordResponse, Message: err.Error(), Retryable: true}, exitTemporary
	}

	return errorView{Code: errorCodeInternal, Message: err.Error(), Retryable: false}, exitUnexpected
}

func discordHTTPErrorForOutput(err error) (httputil.HTTPError, bool) {
	var pointer *httputil.HTTPError
	if errors.As(err, &pointer) && pointer != nil {
		return *pointer, true
	}
	var value httputil.HTTPError
	if errors.As(err, &value) {
		return value, true
	}
	return httputil.HTTPError{}, false
}

func classifyDiscordHTTPError(err error, httpErr httputil.HTTPError) (errorView, int) {
	view := errorView{
		Code:      errorCodeDiscord,
		Message:   err.Error(),
		Retryable: false,
		Details: map[string]any{
			"http_status": httpErr.Status,
		},
	}
	if httpErr.Code != 0 {
		view.Details["discord_code"] = uint(httpErr.Code)
	}

	switch httpErr.Status {
	case http.StatusUnauthorized:
		view.Code = errorCodeAuthenticationFailed
		return view, exitAuthentication
	case http.StatusForbidden:
		view.Code = errorCodePermissionDenied
		return view, exitPermissionDenied
	case http.StatusNotFound:
		view.Code = errorCodeNotFound
		return view, exitNotFound
	case http.StatusConflict:
		view.Code = errorCodeConflict
		return view, exitPrecondition
	case http.StatusTooManyRequests:
		view.Code = errorCodeRateLimited
		view.Retryable = true
		return view, exitTemporary
	default:
		if httpErr.Status >= http.StatusInternalServerError {
			view.Code = errorCodeDiscordUnavailable
			view.Retryable = true
			return view, exitTemporary
		}
		return view, exitUnexpected
	}
}

package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/ayn2op/arikawa/v3/utils/httputil"
)

func TestSanitizedRemoteAuthErrorRedactsCaptchaChallenge(t *testing.T) {
	t.Parallel()

	const secret = "sensitive-rqtoken"
	err := sanitizedRemoteAuthError(httputil.HTTPError{
		Status: 400,
		Body: []byte(`{
			"captcha_key":["captcha-required"],
			"captcha_sitekey":"site-key",
			"captcha_session_id":"session-id",
			"captcha_rqdata":"sensitive-rqdata",
			"captcha_rqtoken":"` + secret + `"
		}`),
	})
	if !errors.Is(err, ErrCaptchaRequired) {
		t.Fatalf("error = %v, want ErrCaptchaRequired", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "sensitive-rqdata") {
		t.Fatalf("captcha challenge leaked in error: %v", err)
	}
}

func TestSanitizedRemoteAuthErrorHidesOtherHTTPBodies(t *testing.T) {
	t.Parallel()

	err := sanitizedRemoteAuthError(httputil.HTTPError{
		Status: 400,
		Body:   []byte(`{"secret":"do-not-print"}`),
	})
	if strings.Contains(err.Error(), "do-not-print") {
		t.Fatalf("HTTP body leaked in error: %v", err)
	}
	if got := err.Error(); got != "Discord returned HTTP 400" {
		t.Fatalf("error = %q", got)
	}
}

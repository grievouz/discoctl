package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ayn2op/arikawa/v3/utils/httputil"
)

func TestCaptchaChallengeAndRetryHeaders(t *testing.T) {
	t.Parallel()

	httpErr := httputil.HTTPError{
		Status: 400,
		Body: []byte(`{
			"captcha_key":["captcha-required"],
			"captcha_service":"hcaptcha",
			"captcha_sitekey":"site-key",
			"captcha_session_id":"session-id",
			"captcha_rqdata":"rqdata",
			"captcha_rqtoken":"rqtoken"
		}`),
	}
	challenge, ok := captchaChallengeFromHTTPError(httpErr)
	if !ok {
		t.Fatal("CAPTCHA challenge was not detected")
	}
	if challenge.Sitekey != "site-key" || challenge.RQData != "rqdata" {
		t.Fatalf("unexpected challenge: %#v", challenge)
	}

	headers := captchaRequestHeaders(challenge, "solution")
	want := map[string]string{
		"X-Captcha-Key":        "solution",
		"X-Captcha-Rqtoken":    "rqtoken",
		"X-Captcha-Session-Id": "session-id",
	}
	for name, value := range want {
		if got := headers.Get(name); got != value {
			t.Fatalf("%s = %q, want %q", name, got, value)
		}
	}
}

func TestCaptchaChallengeFromPointerHTTPError(t *testing.T) {
	t.Parallel()

	err := &httputil.HTTPError{
		Status: 400,
		Body: []byte(`{
			"captcha_key":["captcha-required"],
			"captcha_service":"hcaptcha",
			"captcha_sitekey":"site-key"
		}`),
	}
	challenge, ok := captchaChallengeFromError(err)
	if !ok || challenge.Sitekey != "site-key" {
		t.Fatalf("challenge = %#v, detected = %t", challenge, ok)
	}
	if sanitized := sanitizedRemoteAuthError(err); sanitized != ErrCaptchaRequired {
		t.Fatalf("sanitized error = %v", sanitized)
	}
}

func TestCaptchaPageKeepsRetrySecretsOutOfBrowser(t *testing.T) {
	t.Parallel()

	results := make(chan captchaBrowserResult, 1)
	handler := newCaptchaHandler(captchaChallenge{
		Sitekey:   "site-key",
		RQData:    "browser-rqdata",
		RQToken:   "server-only-rqtoken",
		SessionID: "server-only-session",
	}, "state", "/challenge/", newCaptchaHostSet("captcha.local"), results)

	request := httptest.NewRequest(http.MethodGet, "http://captcha.local/challenge/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	for _, value := range []string{"site-key", "browser-rqdata", "state"} {
		if !strings.Contains(body, value) {
			t.Fatalf("page omitted %q", value)
		}
	}
	for _, secret := range []string{"server-only-rqtoken", "server-only-session"} {
		if strings.Contains(body, secret) {
			t.Fatalf("page exposed %q", secret)
		}
	}

	renderAt := strings.Index(body, "hcaptcha.render")
	setDataAt := strings.Index(body, "hcaptcha.setData")
	if renderAt == -1 || setDataAt == -1 || setDataAt < renderAt {
		t.Fatal("page did not bind rqdata after rendering the hCaptcha widget")
	}
	if !strings.Contains(body, "Submit to Discord") {
		t.Fatal("page omitted the explicit CAPTCHA submission control")
	}
}

func TestCaptchaHandlerAcceptsOneMatchingResponse(t *testing.T) {
	t.Parallel()

	results := make(chan captchaBrowserResult, 1)
	handler := newCaptchaHandler(
		captchaChallenge{Sitekey: "site-key"},
		"correct-state",
		"/challenge/",
		newCaptchaHostSet("captcha.local"),
		results,
	)

	wrongBody, err := json.Marshal(map[string]string{"state": "wrong", "token": "secret"})
	if err != nil {
		t.Fatal(err)
	}
	wrongRequest := httptest.NewRequest(
		http.MethodPost,
		"http://captcha.local/challenge/complete",
		bytes.NewReader(wrongBody),
	)
	wrongResponse := httptest.NewRecorder()
	handler.ServeHTTP(wrongResponse, wrongRequest)
	if wrongResponse.Code != http.StatusForbidden {
		t.Fatalf("wrong-state status = %d", wrongResponse.Code)
	}

	correctBody, err := json.Marshal(map[string]string{"state": "correct-state", "token": "solution"})
	if err != nil {
		t.Fatal(err)
	}
	correctRequest := httptest.NewRequest(
		http.MethodPost,
		"http://captcha.local/challenge/complete",
		bytes.NewReader(correctBody),
	)
	correctResponse := httptest.NewRecorder()
	handler.ServeHTTP(correctResponse, correctRequest)
	if correctResponse.Code != http.StatusNoContent {
		t.Fatalf("correct-state status = %d", correctResponse.Code)
	}

	select {
	case result := <-results:
		if result.Err != nil || result.Token != "solution" {
			t.Fatalf("unexpected result: %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for result")
	}
}

func TestCaptchaHandlerRejectsUnexpectedHost(t *testing.T) {
	t.Parallel()

	handler := newCaptchaHandler(
		captchaChallenge{Sitekey: "site-key"},
		"state",
		"/challenge/",
		newCaptchaHostSet("expected.local"),
		make(chan captchaBrowserResult, 1),
	)
	request := httptest.NewRequest(http.MethodGet, "http://unexpected.local/challenge/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestCaptchaHandlerAcceptsAddedPublicHost(t *testing.T) {
	t.Parallel()

	hosts := newCaptchaHostSet("127.0.0.1:1234")
	hosts.Add("public.lhr.life")
	handler := newCaptchaHandler(
		captchaChallenge{Sitekey: "site-key"},
		"state",
		"/secret-path/",
		hosts,
		make(chan captchaBrowserResult, 1),
	)

	request := httptest.NewRequest(http.MethodGet, "https://public.lhr.life/secret-path/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}

	rootRequest := httptest.NewRequest(http.MethodGet, "https://public.lhr.life/", nil)
	rootResponse := httptest.NewRecorder()
	handler.ServeHTTP(rootResponse, rootRequest)
	if rootResponse.Code != http.StatusNotFound {
		t.Fatalf("root status = %d", rootResponse.Code)
	}
}

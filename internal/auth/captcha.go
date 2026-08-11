package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ayn2op/arikawa/v3/utils/httputil"
)

const (
	captchaSolveTimeout    = 110 * time.Second
	maxCaptchaResponseBody = 32 << 10
)

type captchaChallenge struct {
	CaptchaKey     []string `json:"captcha_key"`
	Service        string   `json:"captcha_service"`
	Sitekey        string   `json:"captcha_sitekey"`
	SessionID      string   `json:"captcha_session_id"`
	RQData         string   `json:"captcha_rqdata"`
	RQToken        string   `json:"captcha_rqtoken"`
	ServeInvisible bool     `json:"should_serve_invisible"`
}

type captchaBrowserResult struct {
	Token string
	Err   error
}

type captchaPageData struct {
	Sitekey   string
	RQData    string
	State     string
	Invisible bool
}

type captchaHostSet struct {
	mu    sync.RWMutex
	hosts map[string]struct{}
}

func newCaptchaHostSet(hosts ...string) *captchaHostSet {
	set := &captchaHostSet{hosts: make(map[string]struct{}, len(hosts))}
	for _, host := range hosts {
		set.Add(host)
	}
	return set
}

func (s *captchaHostSet) Add(host string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hosts[strings.ToLower(host)] = struct{}{}
}

func (s *captchaHostSet) Allows(host string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.hosts[strings.ToLower(host)]
	return ok
}

var captchaPage = template.Must(template.New("captcha").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Discoctl CAPTCHA</title>
  <style>
    :root { color-scheme: dark; font-family: system-ui, sans-serif; }
    body { min-height: 100vh; margin: 0; display: grid; place-items: center; background: #1e1f22; color: #f2f3f5; }
    main { width: min(92vw, 520px); padding: 28px; border-radius: 14px; background: #2b2d31; box-shadow: 0 16px 48px #0008; }
    h1 { margin-top: 0; font-size: 1.35rem; }
    p { color: #b5bac1; line-height: 1.45; }
    #status { margin-top: 18px; color: #dbdee1; }
    button { margin-top: 16px; padding: 10px 16px; border: 0; border-radius: 6px; background: #5865f2; color: white; font: inherit; cursor: pointer; }
    button:disabled { cursor: default; opacity: .6; }
  </style>
  <script>
    let captchaToken = "";
    function finish(path, body) {
      return fetch(path, {
        method: "POST",
        headers: {"Content-Type": "application/json"},
        body: JSON.stringify(body)
      });
    }
    function captchaSolved(token) {
      captchaToken = token;
      document.getElementById("status").textContent = "Challenge solved. Submit it to Discord when you are ready.";
      document.getElementById("submit").hidden = false;
    }
    async function submitCaptcha() {
      if (!captchaToken) return;
      const button = document.getElementById("submit");
      button.disabled = true;
      document.getElementById("status").textContent = "Submitting to Discord…";
      try {
        await finish("complete", {
          state: document.getElementById("captcha").dataset.state,
          token: captchaToken
        });
        document.getElementById("status").textContent = "Submitted. You can close this tab.";
      } catch (_) {
        button.disabled = false;
        document.getElementById("status").textContent = "Could not reach Discoctl. Try submitting again.";
      }
    }
    function captchaFailed(code) {
      document.getElementById("status").textContent = "Challenge failed: " + code;
      finish("failed", {state: document.getElementById("captcha").dataset.state, error: String(code)});
    }
    function captchaLoaded() {
      const element = document.getElementById("captcha");
      const options = {
        sitekey: element.dataset.sitekey,
        callback: captchaSolved,
        "error-callback": captchaFailed,
        "expired-callback": () => captchaFailed("expired"),
        "chalexpired-callback": () => captchaFailed("challenge-expired")
      };
      if (element.dataset.invisible === "true") options.size = "invisible";
      const widget = hcaptcha.render(element, options);
      if (element.dataset.rqdata) {
        hcaptcha.setData(widget, {rqdata: element.dataset.rqdata});
      }
      if (element.dataset.invisible === "true") hcaptcha.execute(widget);
      else document.getElementById("status").textContent = "Complete the challenge above.";
    }
  </script>
  <script src="https://js.hcaptcha.com/1/api.js?onload=captchaLoaded&render=explicit&recaptchacompat=off" async defer></script>
</head>
<body>
  <main>
    <h1>Complete Discord's CAPTCHA</h1>
    <p>This human-solved challenge is returned only to the waiting Discoctl process.</p>
    <div id="captcha" data-sitekey="{{.Sitekey}}" data-rqdata="{{.RQData}}" data-state="{{.State}}" data-invisible="{{.Invisible}}"></div>
    <div id="status">Loading challenge…</div>
    <button id="submit" type="button" onclick="submitCaptcha()" hidden>Submit to Discord</button>
  </main>
</body>
</html>`))

func captchaChallengeFromError(err error) (captchaChallenge, bool) {
	httpErr, ok := discordHTTPError(err)
	if !ok {
		return captchaChallenge{}, false
	}
	return captchaChallengeFromHTTPError(httpErr)
}

func discordHTTPError(err error) (httputil.HTTPError, bool) {
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

func captchaChallengeFromHTTPError(httpErr httputil.HTTPError) (captchaChallenge, bool) {
	var challenge captchaChallenge
	if json.Unmarshal(httpErr.Body, &challenge) != nil || challenge.Sitekey == "" {
		return captchaChallenge{}, false
	}
	for _, key := range challenge.CaptchaKey {
		if key == "captcha-required" {
			return challenge, true
		}
	}
	return captchaChallenge{}, false
}

func captchaRequestHeaders(challenge captchaChallenge, solution string) http.Header {
	headers := make(http.Header)
	headers.Set("X-Captcha-Key", solution)
	if challenge.RQToken != "" {
		headers.Set("X-Captcha-Rqtoken", challenge.RQToken)
	}
	if challenge.SessionID != "" {
		headers.Set("X-Captcha-Session-Id", challenge.SessionID)
	}
	return headers
}

func solveCaptcha(
	ctx context.Context,
	out io.Writer,
	challenge captchaChallenge,
	publicTunnel bool,
) (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen on loopback: %w", err)
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		_ = listener.Close()
		return "", fmt.Errorf("generate browser state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)
	localHost := listener.Addr().String()
	_, localPort, err := net.SplitHostPort(localHost)
	if err != nil {
		_ = listener.Close()
		return "", fmt.Errorf("resolve browser listener: %w", err)
	}
	basePath := "/" + state + "/"
	hosts := newCaptchaHostSet(localHost)
	result := make(chan captchaBrowserResult, 1)
	handler := newCaptchaHandler(challenge, state, basePath, hosts, result)
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       15 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	localURL := "http://" + localHost + basePath
	fmt.Fprintf(out, "Local challenge URL: %s\n", localURL)
	challengeURL := localURL
	var tunnel *captchaTunnel
	if publicTunnel {
		fmt.Fprintln(out, "Creating a temporary public challenge URL with localhost.run...")
		tunnel, err = startPublicTunnel(ctx, localPort)
		if err != nil {
			detail := strings.ReplaceAll(err.Error(), "\n", "; ")
			fmt.Fprintf(out, "Could not create a public URL (%s); using loopback.\n", detail)
		} else {
			defer tunnel.Close()
			parsed, parseErr := url.Parse(tunnel.URL)
			if parseErr != nil || parsed.Host == "" {
				return "", errors.New("localhost.run returned an invalid public URL")
			}
			hosts.Add(parsed.Host)
			challengeURL = strings.TrimRight(tunnel.URL, "/") + basePath
			fmt.Fprintf(out, "Public challenge URL: %s\n", challengeURL)
		}
	}

	if err := openBrowser(challengeURL); err != nil {
		fmt.Fprintf(out, "Could not open a browser automatically: %v\n", err)
		fmt.Fprintf(out, "Open this URL manually: %s\n", challengeURL)
	} else {
		fmt.Fprintf(out, "Browser challenge opened at %s\n", challengeURL)
	}

	timer := time.NewTimer(captchaSolveTimeout)
	defer timer.Stop()
	var tunnelDone <-chan error
	if tunnel != nil {
		tunnelDone = tunnel.Done
	}
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
			return "", errors.New("browser challenge timed out")
		case err := <-serveErr:
			return "", fmt.Errorf("serve browser challenge: %w", err)
		case err := <-tunnelDone:
			tunnelDone = nil
			if err != nil {
				fmt.Fprintf(out, "Public challenge tunnel closed (%v); the local URL remains available.\n", err)
			} else {
				fmt.Fprintln(out, "Public challenge tunnel closed; the local URL remains available.")
			}
		case solved := <-result:
			if solved.Err != nil {
				return "", solved.Err
			}
			if solved.Token == "" {
				return "", errors.New("browser returned an empty CAPTCHA response")
			}
			return solved.Token, nil
		}
	}
}

func newCaptchaHandler(
	challenge captchaChallenge,
	state string,
	basePath string,
	hosts *captchaHostSet,
	result chan<- captchaBrowserResult,
) http.Handler {
	var once sync.Once
	deliver := func(value captchaBrowserResult) {
		once.Do(func() { result <- value })
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+basePath, func(w http.ResponseWriter, r *http.Request) {
		setCaptchaHeaders(w.Header())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if !hosts.Allows(r.Host) {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		if err := captchaPage.Execute(w, captchaPageData{
			Sitekey:   challenge.Sitekey,
			RQData:    challenge.RQData,
			State:     state,
			Invisible: challenge.ServeInvisible,
		}); err != nil {
			deliver(captchaBrowserResult{Err: fmt.Errorf("render challenge page: %w", err)})
		}
	})
	mux.HandleFunc("POST "+basePath+"complete", func(w http.ResponseWriter, r *http.Request) {
		setCaptchaHeaders(w.Header())
		if !hosts.Allows(r.Host) {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		var request struct {
			State string `json:"state"`
			Token string `json:"token"`
		}
		if err := decodeCaptchaRequest(w, r, &request); err != nil {
			return
		}
		if request.State != state || strings.TrimSpace(request.Token) == "" {
			http.Error(w, "invalid response", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		deliver(captchaBrowserResult{Token: request.Token})
	})
	mux.HandleFunc("POST "+basePath+"failed", func(w http.ResponseWriter, r *http.Request) {
		setCaptchaHeaders(w.Header())
		if !hosts.Allows(r.Host) {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		var request struct {
			State string `json:"state"`
			Error string `json:"error"`
		}
		if err := decodeCaptchaRequest(w, r, &request); err != nil {
			return
		}
		if request.State != state {
			http.Error(w, "invalid response", http.StatusForbidden)
			return
		}
		detail := strings.TrimSpace(request.Error)
		if len(detail) > 128 {
			detail = detail[:128]
		}
		w.WriteHeader(http.StatusNoContent)
		deliver(captchaBrowserResult{Err: fmt.Errorf("browser challenge failed: %s", detail)})
	})
	return mux
}

func decodeCaptchaRequest(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxCaptchaResponseBody)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(destination); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return err
	}
	return nil
}

func setCaptchaHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline' https://js.hcaptcha.com https://*.hcaptcha.com; frame-src https://*.hcaptcha.com https://hcaptcha.com; connect-src 'self' https://*.hcaptcha.com https://hcaptcha.com; style-src 'unsafe-inline' https://*.hcaptcha.com https://hcaptcha.com; img-src data: https://*.hcaptcha.com https://hcaptcha.com")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
}

func openBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

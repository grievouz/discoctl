package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ayn2op/arikawa/v3/api"
	"github.com/ayn2op/arikawa/v3/utils/httputil"
	"github.com/gorilla/websocket"
	clientgateway "github.com/grievouz/discoctl/internal/gateway"
	clienthttp "github.com/grievouz/discoctl/internal/http"
	"github.com/skip2/go-qrcode"
)

const remoteAuthGatewayURL = "wss://remote-auth-gateway.discord.gg/?v=2"

type remoteAuthEnvelope struct {
	Op string `json:"op"`
}

var ErrCaptchaRequired = errors.New("Discord requires a CAPTCHA for this login")

type QRLoginOptions struct {
	LocalCaptchaOnly bool
}

func LoginQR(ctx context.Context, out io.Writer, options QRLoginOptions) (string, error) {
	headers := clienthttp.Headers()
	headers.Set("User-Agent", clienthttp.BrowserUserAgent())

	dialer := clientgateway.NewDialer()
	conn, _, err := dialer.DialContext(ctx, remoteAuthGatewayURL, headers)
	if err != nil {
		return "", fmt.Errorf("connect to remote authentication gateway: %w", err)
	}
	defer conn.Close()

	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("generate QR login key: %w", err)
	}

	var (
		fingerprint     string
		writeMu         sync.Mutex
		heartbeatActive bool
	)
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	defer stopHeartbeat()

	writeJSON := func(v any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(v)
	}

	fmt.Fprintln(out, "Connecting to Discord QR login...")
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", fmt.Errorf("read remote authentication event: %w", err)
		}

		var envelope remoteAuthEnvelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			return "", fmt.Errorf("decode remote authentication event: %w", err)
		}

		switch envelope.Op {
		case "hello":
			var payload struct {
				HeartbeatInterval int `json:"heartbeat_interval"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				return "", fmt.Errorf("decode QR login hello: %w", err)
			}
			if payload.HeartbeatInterval <= 0 {
				return "", errors.New("QR login returned an invalid heartbeat interval")
			}
			if !heartbeatActive {
				heartbeatActive = true
				go heartbeat(heartbeatCtx, conn, &writeMu, time.Duration(payload.HeartbeatInterval)*time.Millisecond)
			}
			if err := sendInit(writeJSON, privateKey); err != nil {
				return "", err
			}

		case "nonce_proof":
			var payload struct {
				EncryptedNonce string `json:"encrypted_nonce"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				return "", fmt.Errorf("decode QR login nonce: %w", err)
			}
			if err := sendNonceProof(writeJSON, privateKey, payload.EncryptedNonce); err != nil {
				return "", err
			}

		case "pending_remote_init":
			var payload struct {
				Fingerprint string `json:"fingerprint"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				return "", fmt.Errorf("decode QR login fingerprint: %w", err)
			}
			fingerprint = payload.Fingerprint
			if err := printQRCode(out, fingerprint); err != nil {
				return "", err
			}

		case "pending_ticket":
			var payload struct {
				EncryptedUserPayload string `json:"encrypted_user_payload"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				return "", fmt.Errorf("decode QR login user: %w", err)
			}
			username, err := decryptUsername(privateKey, payload.EncryptedUserPayload)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(out, "Confirm the login for %s in the Discord mobile app.\n", username)

		case "pending_login":
			var payload struct {
				Ticket string `json:"ticket"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				return "", fmt.Errorf("decode QR login ticket: %w", err)
			}
			return exchangeTicket(ctx, out, fingerprint, privateKey, payload.Ticket, options)

		case "cancel":
			return "", errors.New("QR login was canceled in the Discord mobile app")
		}
	}
}

func heartbeat(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			writeMu.Lock()
			err := conn.WriteJSON(remoteAuthEnvelope{Op: "heartbeat"})
			writeMu.Unlock()
			if err != nil {
				_ = conn.Close()
				return
			}
		}
	}
}

func sendInit(writeJSON func(any) error, privateKey *rsa.PrivateKey) error {
	spki, err := x509.MarshalPKIXPublicKey(privateKey.Public())
	if err != nil {
		return fmt.Errorf("marshal QR login public key: %w", err)
	}
	payload := struct {
		Op               string `json:"op"`
		EncodedPublicKey string `json:"encoded_public_key"`
	}{
		Op:               "init",
		EncodedPublicKey: base64.StdEncoding.EncodeToString(spki),
	}
	if err := writeJSON(payload); err != nil {
		return fmt.Errorf("initialize QR login: %w", err)
	}
	return nil
}

func sendNonceProof(writeJSON func(any) error, privateKey *rsa.PrivateKey, encryptedNonce string) error {
	decodedNonce, err := base64.StdEncoding.DecodeString(encryptedNonce)
	if err != nil {
		return fmt.Errorf("decode QR login nonce: %w", err)
	}
	decryptedNonce, err := rsa.DecryptOAEP(sha256.New(), nil, privateKey, decodedNonce, nil)
	if err != nil {
		return fmt.Errorf("decrypt QR login nonce: %w", err)
	}
	payload := struct {
		Op    string `json:"op"`
		Nonce string `json:"nonce"`
	}{
		Op:    "nonce_proof",
		Nonce: base64.RawURLEncoding.EncodeToString(decryptedNonce),
	}
	if err := writeJSON(payload); err != nil {
		return fmt.Errorf("send QR login nonce proof: %w", err)
	}
	return nil
}

func printQRCode(out io.Writer, fingerprint string) error {
	code, err := qrcode.New("https://discord.com/ra/"+fingerprint, qrcode.Low)
	if err != nil {
		return fmt.Errorf("generate QR code: %w", err)
	}
	code.DisableBorder = true

	fmt.Fprintln(out, "Scan this QR code with the Discord mobile app:")
	bitmap := code.Bitmap()
	for y := 0; y < len(bitmap); y += 2 {
		var line strings.Builder
		for x := range bitmap[y] {
			top := bitmap[y][x]
			bottom := y+1 < len(bitmap) && bitmap[y+1][x]
			line.WriteRune(halfBlock(top, bottom))
		}
		fmt.Fprintln(out, line.String())
	}
	return nil
}

func halfBlock(top, bottom bool) rune {
	switch [2]bool{top, bottom} {
	case [2]bool{true, true}:
		return '█'
	case [2]bool{true, false}:
		return '▀'
	case [2]bool{false, true}:
		return '▄'
	default:
		return ' '
	}
}

func decryptUsername(privateKey *rsa.PrivateKey, encryptedPayload string) (string, error) {
	decodedPayload, err := base64.StdEncoding.DecodeString(encryptedPayload)
	if err != nil {
		return "", fmt.Errorf("decode QR login user: %w", err)
	}
	decryptedPayload, err := rsa.DecryptOAEP(sha256.New(), nil, privateKey, decodedPayload, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt QR login user: %w", err)
	}

	parts := strings.Split(string(decryptedPayload), ":")
	if len(parts) != 4 {
		return "", errors.New("QR login returned an invalid user payload")
	}
	username := parts[3]
	if parts[1] != "0" {
		username += "#" + parts[1]
	}
	return username, nil
}

func exchangeTicket(
	ctx context.Context,
	out io.Writer,
	fingerprint string,
	privateKey *rsa.PrivateKey,
	ticket string,
	options QRLoginOptions,
) (string, error) {
	headers := clienthttp.Headers()
	headers.Set("Referer", "https://discord.com/login")
	if fingerprint != "" {
		headers.Set("X-Fingerprint", fingerprint)
	}

	client := clienthttp.NewClient("")
	client.OnRequest = append(client.OnRequest, httputil.WithHeaders(headers))
	encryptedToken, err := exchangeRemoteAuthTicket(client, ticket, nil)
	if err != nil {
		challenge, ok := captchaChallengeFromError(err)
		if ok {
			if challenge.Service != "" && challenge.Service != "hcaptcha" {
				return "", fmt.Errorf("exchange QR login ticket: unsupported CAPTCHA service %q", challenge.Service)
			}
			fmt.Fprintln(out, "Discord requires a CAPTCHA. Opening a one-time browser challenge...")
			solution, solveErr := solveCaptcha(ctx, out, challenge, !options.LocalCaptchaOnly)
			if solveErr != nil {
				return "", fmt.Errorf("solve QR login CAPTCHA: %w", solveErr)
			}

			captchaHeaders := captchaRequestHeaders(challenge, solution)
			encryptedToken, err = exchangeRemoteAuthTicket(client, ticket, captchaHeaders)
			solution = ""
		}
	}
	if err != nil {
		return "", fmt.Errorf("exchange QR login ticket: %w", sanitizedRemoteAuthError(err))
	}
	decodedToken, err := base64.StdEncoding.DecodeString(encryptedToken)
	if err != nil {
		return "", fmt.Errorf("decode QR login token: %w", err)
	}
	token, err := rsa.DecryptOAEP(sha256.New(), nil, privateKey, decodedToken, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt QR login token: %w", err)
	}
	return string(token), nil
}

func exchangeRemoteAuthTicket(client *api.Client, ticket string, headers http.Header) (string, error) {
	body := struct {
		Ticket string `json:"ticket"`
	}{Ticket: ticket}
	var response struct {
		EncryptedToken string `json:"encrypted_token"`
	}
	options := []httputil.RequestOption{httputil.WithJSONBody(body)}
	if len(headers) > 0 {
		options = append(options, httputil.WithHeaders(headers))
	}
	err := client.RequestJSON(
		&response,
		"POST",
		api.EndpointRemoteAuthLogin,
		options...,
	)
	return response.EncryptedToken, err
}

func sanitizedRemoteAuthError(err error) error {
	httpErr, ok := discordHTTPError(err)
	if !ok {
		return err
	}

	if _, ok := captchaChallengeFromHTTPError(httpErr); ok {
		return ErrCaptchaRequired
	}
	if httpErr.Code > 0 {
		return fmt.Errorf("Discord returned HTTP %d error code %d", httpErr.Status, httpErr.Code)
	}
	return fmt.Errorf("Discord returned HTTP %d", httpErr.Status)
}

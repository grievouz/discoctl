package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const publicTunnelStartupTimeout = 30 * time.Second

var publicTunnelURLPattern = regexp.MustCompile(
	`https://[[:alnum:]][[:alnum:].-]*\.(?:lhr\.life|localhost\.run)(?::[0-9]+)?`,
)

type captchaTunnel struct {
	URL    string
	Done   <-chan error
	cancel context.CancelFunc
}

func (t *captchaTunnel) Close() {
	t.cancel()
	select {
	case <-t.Done:
	case <-time.After(time.Second):
	}
}

func startPublicTunnel(ctx context.Context, localPort string) (*captchaTunnel, error) {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return nil, errors.New("OpenSSH client not found")
	}

	return startLocalhostRunTunnel(ctx, sshPath, localPort)
}

func startLocalhostRunTunnel(
	ctx context.Context,
	sshPath string,
	localPort string,
) (*captchaTunnel, error) {
	attemptCtx, cancel := context.WithCancel(ctx)
	arguments := []string{
		"-T",
		"-o", "BatchMode=yes",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "IdentityAgent=none",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=2",
		"-o", "StrictHostKeyChecking=accept-new",
		"-R", "80:127.0.0.1:" + localPort,
		"nokey@localhost.run",
	}
	command := exec.CommandContext(attemptCtx, sshPath, arguments...)
	reader, writer := io.Pipe()
	command.Stdout = writer
	command.Stderr = writer
	if err := command.Start(); err != nil {
		cancel()
		_ = reader.Close()
		_ = writer.Close()
		return nil, err
	}

	done := make(chan error, 1)
	go func() {
		err := command.Wait()
		_ = writer.CloseWithError(err)
		done <- err
		close(done)
	}()

	urls := make(chan string, 1)
	tails := make(chan string, 1)
	go scanTunnelOutput(reader, urls, tails)

	timer := time.NewTimer(publicTunnelStartupTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	case <-timer.C:
		cancel()
		return nil, errors.New("timed out waiting for a public URL")
	case err := <-done:
		cancel()
		tail := <-tails
		if err == nil {
			err = errors.New("SSH exited before assigning a public URL")
		}
		if tail != "" {
			return nil, fmt.Errorf("%w: %s", err, tail)
		}
		return nil, err
	case publicURL := <-urls:
		return &captchaTunnel{
			URL:    publicURL,
			Done:   done,
			cancel: cancel,
		}, nil
	}
}

func scanTunnelOutput(reader io.Reader, urls chan<- string, tails chan<- string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 64<<10)
	sent := false
	var tail []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			tail = append(tail, line)
			if len(tail) > 4 {
				tail = tail[len(tail)-4:]
			}
		}
		if sent {
			continue
		}
		if publicURL, ok := publicTunnelURLFromText(line); ok {
			urls <- publicURL
			sent = true
		}
	}
	detail := strings.Join(tail, "; ")
	if len(detail) > 1024 {
		detail = detail[len(detail)-1024:]
	}
	tails <- detail
}

func publicTunnelURLFromText(text string) (string, bool) {
	if !strings.Contains(strings.ToLower(text), "tunneled with tls termination") {
		return "", false
	}
	match := publicTunnelURLPattern.FindString(text)
	if match == "" {
		return "", false
	}
	parsed, err := url.Parse(match)
	if err != nil || parsed.Scheme != "https" {
		return "", false
	}
	hostname := strings.ToLower(parsed.Hostname())
	if !validPublicTunnelHostname(hostname) {
		return "", false
	}
	return "https://" + parsed.Host, true
}

func validPublicTunnelHostname(hostname string) bool {
	for _, suffix := range []string{".lhr.life", ".localhost.run"} {
		if strings.HasSuffix(hostname, suffix) && hostname != suffix {
			return true
		}
	}
	return false
}

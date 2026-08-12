package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/grievouz/discoctl/internal/auth"
)

const maxTokenBytes = 16 << 10

func Run() error {
	return RunContext(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

func RunContext(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stdout)
		return nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	case "version", "-version", "--version":
		fmt.Fprintln(stdout, buildVersion())
		return nil
	case "auth":
		return runAuth(ctx, args[1:], stdin, stdout, stderr)
	case "guilds":
		return runGuilds(ctx, args[1:], stdout, stderr)
	case "channels":
		return runChannels(ctx, args[1:], stdout, stderr)
	case "messages":
		return runMessages(ctx, args[1:], stdin, stdout, stderr)
	case "inbox":
		return runInbox(ctx, args[1:], stdout, stderr)
	case "emojis":
		return runEmojis(ctx, args[1:], stdout, stderr)
	case "reactions":
		return runReactions(ctx, args[1:], stdout, stderr)
	case "profile":
		return runProfile(ctx, args[1:], stdout, stderr)
	case "status":
		return runStatus(ctx, args[1:], stdout, stderr)
	default:
		return invalidArgumentsf("unknown command %q; run 'discoctl help'", args[0])
	}
}

func runAuth(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printAuthUsage(stdout)
		return nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		printAuthUsage(stdout)
		return nil
	case "login":
		return runAuthLogin(ctx, args[1:], stdin, stdout, stderr)
	case "status":
		if len(args) != 1 {
			return invalidArguments(errors.New("auth status does not accept arguments"))
		}
		_, source, err := auth.LoadToken()
		if err != nil {
			if errors.Is(err, auth.ErrNoToken) {
				fmt.Fprintln(stdout, "not authenticated")
				return nil
			}
			return err
		}
		fmt.Fprintf(stdout, "authenticated via %s\n", source)
		return nil
	case "logout":
		if len(args) != 1 {
			return invalidArguments(errors.New("auth logout does not accept arguments"))
		}
		if err := auth.DeleteToken(); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "removed stored credentials")
		return nil
	default:
		return invalidArgumentsf("unknown auth command %q; run 'discoctl auth help'", args[0])
	}
}

func runAuthLogin(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("auth login", flag.ContinueOnError)
	flags.SetOutput(stderr)
	tokenValue := flags.String("token", "", "import a token passed as an argument (visible in shell history and process listings)")
	tokenStdin := flags.Bool("token-stdin", false, "read a token from standard input instead of using QR login")
	localCaptcha := flags.Bool("local-captcha", false, "keep a QR login CAPTCHA on loopback instead of creating a public URL")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return invalidArguments(errors.New("auth login does not accept positional arguments"))
	}

	if *tokenValue != "" && *tokenStdin {
		return invalidArguments(errors.New("auth login accepts only one of --token or --token-stdin"))
	}
	if *localCaptcha && (*tokenValue != "" || *tokenStdin) {
		return invalidArguments(errors.New("--local-captcha can only be used with QR login"))
	}

	var (
		token string
		err   error
	)
	switch {
	case *tokenValue != "":
		token = strings.TrimSpace(*tokenValue)
		if token == "" {
			return invalidArguments(errors.New("token is empty"))
		}
	case *tokenStdin:
		token, err = readToken(stdin)
	default:
		token, err = auth.LoginQR(ctx, stdout, auth.QRLoginOptions{
			LocalCaptchaOnly: *localCaptcha,
		})
	}
	if err != nil {
		return err
	}
	if err := auth.SaveToken(token); err != nil {
		return err
	}

	fmt.Fprintln(stdout, "authenticated; token stored in the system keyring")
	return nil
}

func readToken(r io.Reader) (string, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxTokenBytes+1))
	if err != nil {
		return "", fmt.Errorf("read token: %w", err)
	}
	if len(b) > maxTokenBytes {
		return "", invalidArguments(errors.New("token exceeds maximum size"))
	}
	token := strings.TrimSpace(string(b))
	if token == "" {
		return "", invalidArguments(errors.New("token is empty"))
	}
	return token, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: discoctl <command>

Commands:
  auth       Manage authentication
  guilds     List accessible guilds
  channels   Inspect guild channels
  messages   Read channel message history
  inbox      Inspect account-wide unread state
  emojis     List guild emoji syntax
  reactions  Add message reactions
  profile    Show or update the account profile
  status     Show or update presence and custom status
  version    Print the version
  help       Show this help`)
}

func printAuthUsage(w io.Writer) {
	fmt.Fprintln(w, `Usage: discoctl auth <command>

Commands:
  login      Authenticate with a QR code or token
  status     Show whether credentials are available
  logout     Remove credentials from the system keyring

Use 'discoctl auth login --token <token>' for direct argument import. This can
expose the token in shell history and process listings; --token-stdin avoids that.
QR login opens a human-solved browser challenge when required. By default it
also creates a temporary public localhost.run URL; --local-captcha keeps it local.`)
}

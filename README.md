# Discoctl

Discoctl is a scriptable command-line client for reading and interacting with Discord conversations. It is designed for both people and software agents, with stable structured output and explicit state-changing commands.

> [!IMPORTANT]
> Automating a normal Discord user account is against Discord's Terms of Service and may result in account termination. Discoctl is not affiliated with or endorsed by Discord.

Discoctl is under active development. Its command output is JSON by default so it can be consumed without scraping terminal UI output.

## Installation

After the first tagged release is published, install the current release from Homebrew:

```sh
brew install grievouz/tap/discoctl
```

Before then, or to build the current `main` branch, use the tap's HEAD formula:

```sh
brew install grievouz/tap/discoctl --HEAD
```

To install a local checkout under `~/.local/bin`:

```sh
make install
```

## Build

```sh
make build
```

## Releasing

The release workflow expects a repository secret named `HOMEBREW_TAP_TOKEN`. Use a fine-grained GitHub token with repository contents read/write access limited to `grievouz/homebrew-tap`.

Publish a GitHub release whose tag follows `vX.Y.Z`. The workflow tests the project, builds macOS and Linux archives for arm64 and x86-64, uploads checksums, and updates `Formula/discoctl.rb` in the tap automatically.

## Authentication

Log in by scanning a QR code with the Discord mobile app:

```sh
discoctl auth login
```

Import a token through standard input without placing it in shell history:

```sh
printf '%s' "$DISCORD_TOKEN" | discoctl auth login --token-stdin
```

Or pass it directly as an argument:

```sh
discoctl auth login --token "$DISCORD_TOKEN"
```

The direct argument form can expose the token in shell history and process listings.

Check or clear the stored credentials:

```sh
discoctl auth status
discoctl auth logout
```

For an ephemeral session, set `DISCOCTL_TOKEN`; it takes precedence over the token stored in the system keyring.

Discord may apply a CAPTCHA risk challenge to the final QR ticket exchange. Discoctl automatically opens a one-time, human-solved browser page when this happens; it does not automate solving the CAPTCHA.

By default, Discoctl uses the system `ssh` client to request a temporary HTTPS URL from localhost.run. Open the printed public URL on any device, complete the challenge, and choose **Submit to Discord**. The temporary tunnel and local listener close as soon as the challenge completes or times out.

Keep the challenge on loopback when a public handoff is not wanted:

```sh
discoctl auth login --local-captcha
```

The public URL contains a random one-time path. The Discord authentication token, CAPTCHA request token, and CAPTCHA session ID stay inside Discoctl, but the challenge page and one-time hCaptcha response pass through localhost.run, which [terminates HTTPS for HTTP tunnels](https://localhost.run/docs/security/). If OpenSSH or localhost.run is unavailable, Discoctl prints and opens the loopback URL instead.

## Reading Discord

```sh
discoctl guilds list --json
discoctl channels list --guild <guild-id> --json
discoctl messages list --channel <channel-id> --limit 50 --json
discoctl messages get --channel <channel-id> --message <message-id> --json
discoctl inbox unread --json
discoctl inbox unread --all --json
discoctl emojis list --guild <guild-id> --json
```

`inbox unread` is mentions-only by default, including mentions in muted channels. Add `--all` for ordinary unread channels and `--messages` for a bounded context window. Ordinary unread DMs appear with `--all`. Fetching messages never marks them read.

Message history is emitted oldest-to-newest and includes pagination cursors. Discord snowflakes are always JSON strings.

## Sending and replying

```sh
discoctl messages send --channel <channel-id> --msg "hello"
discoctl messages reply --channel <channel-id> --message <message-id> --text "hello"
printf '%s' "hello" | discoctl messages reply --channel <channel-id> --message <message-id> --stdin
discoctl messages send --channel <channel-id> --msg "hello" --dry-run
discoctl reactions add --channel <channel-id> --message <message-id> --emoji "👍"
```

Discord-style mention parsing is enabled by default, and replies ping their author. Use `--no-mentions`, one of the granular `--no-*-mentions` flags, or `--no-ping-reply-author` when those notifications are unwanted. Pass `--nonce` when a caller needs a stable client nonce across retries.

Sends and replies fail closed unless the channel's read cursor is at or beyond its latest message. Use `--ignore-unread` to bypass that guard explicitly. The check is a snapshot immediately before sending; Discord does not provide an atomic check-and-send operation.

Explicit channel and message IDs are canonical for message actions. Discord message URLs and `channelID/messageID` references remain accepted as conveniences. Reaction writes use the same unread guard.

## Profile and status

Inspect or update the account profile:

```sh
discoctl profile show
discoctl profile update --bio "building tiny tools" --color '#FF69B4' --dry-run
discoctl profile update --avatar ./avatar.png
discoctl profile update --banner ./banner.png
discoctl profile update --clear-bio --clear-color
```

Avatar and banner files must be PNG, JPEG, or GIF and no larger than 10 MiB. Multi-field updates are not atomic, so `--dry-run` validates local inputs before connecting. Discord's personal-account bio and accent-color route is undocumented and may change independently of Discoctl.

Inspect or update presence and custom status:

```sh
discoctl status show
discoctl status set --presence dnd --text "heads down" --emoji "🌸" --dry-run
discoctl status set --presence invisible
discoctl status clear
```

When `--presence` is omitted, changing or clearing the custom status preserves the current presence.

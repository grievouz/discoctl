# Repository conventions

## Commits

- Use Conventional Commits with the subject format `type: description` or
  `type!: description` for a breaking change.
- Use one of these lowercase types: `build`, `chore`, `ci`, `docs`, `feat`,
  `fix`, `perf`, `refactor`, `revert`, `style`, or `test`.
- Do not use commit scopes. For example, use `fix: preserve selection`, not
  `fix(web): preserve selection`.
- Use a single subject line with no commit body.
- Do not add assistant, agent, tool, or model attribution.
- Do not add `Co-authored-by`, `Generated-by`, or similar trailers for automated work.

## Branches

- Use flat lowercase kebab-case branch names.
- Do not prefix branch names with `codex/`, `agent/`, or another automation identity.
- Do not use slash-separated branch names unless the user explicitly requests one.

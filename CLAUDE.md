# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working in this repository.

The canonical guide for agents and contributors is AGENTS.md, imported
below. Follow it for build/test commands, layout, workflows, coding
rules, commit messages, and communication style.

@AGENTS.md

## Additional References

Read these on demand instead of duplicating their content here:

- `docs/architecture.md` — module responsibilities, startup flow, and
  design boundaries (the up-to-date architecture description).
- `README.md` — user-facing docs: usage, CLI options, controls, book
  mode behavior, and all configuration keys.
- `make help` — full list of build/test/bench targets.

## Claude Code Notes

- The entry point is `startup.go` (`func main`); there is no `main.go`.
  The former `main.go` was split into `startup.go` and the
  `game_*.go` files (see `docs/architecture.md`).
- The "For Agents" section in AGENTS.md mentions Codex CLI tools
  (`update_plan`, `apply_patch`). Use the Claude Code equivalents; the
  workflow intent (plan → small patch → checks) still applies.

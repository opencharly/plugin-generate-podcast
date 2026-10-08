# plugin-generate-podcast

**The podcast desk** — turn a topic plus a set of sources into a two-host discussion, as a script and
(optionally) as audio.

Sources are pluggable: repo files, globs, changelog windows, git ranges, GitHub issues and PRs,
documents/PDFs/URLs, named doctrine sets, and prior episodes. Discussing *the latest code changes* is one
source profile among many, not the purpose.

Every shape is a CUE definition under `candy/plugin-generate-podcast/schema/` — the run config, the
resolved source bundle, the episode, and the render manifest — validated at each boundary. **The script is
data**: the markdown and the renderer's cue lines are emitted from a validated `#Episode`, so a malformed
episode cannot be constructed and nothing is hand-parsed.

## Status

Foundations landed first, by design: the schemas, their two-arm fixture, the generated Go types, and the
reproducibility gate. The provider, its leaves, and the skill follow in the same line of work.

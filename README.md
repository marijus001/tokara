# tokara

Context compression for AI coding tools — a local proxy with smart auto-compaction and a live TUI dashboard.

Tokara sits between your AI coding tools and any LLM API. It monitors context size and automatically compresses conversation history in the background, so your agent sessions never hit context limits.

## Install

```bash
# npm (recommended)
npx tokara-cli

# or download binary directly
curl -fsSL https://github.com/marijus001/tokara/releases/latest/download/tokara-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') -o /usr/local/bin/tokara && chmod +x /usr/local/bin/tokara
```

## Quick Start

```bash
tokara              # start proxy + live TUI dashboard
tokara setup        # detect and configure your AI tools
tokara demo         # run with mock upstreams (no API key needed)
```

The setup wizard detects your AI tools (Claude Code, Codex, Aider, Continue.dev) and configures them to route through the proxy automatically.

## How It Works

```
AI Tool → localhost:18741 (tokara proxy) → LLM API
```

- **Under 60%** of context window: requests pass through untouched
- **At 60%**: background precomputation of compacted context begins
- **At 80%**: precomputed compaction is applied instantly — zero latency

Compression is structure-preserving: function signatures, types, imports, and recent tool outputs are never removed. Old conversational prose is summarized, code blocks are reduced to signatures (4:1 to 8:1 ratio).

## TUI Dashboard

The gateway runs a live terminal dashboard:

| Key | Panel |
|-----|-------|
| `l` | Logs — live stream of proxy events |
| `c` | Config — view and edit settings inline |
| `t` | Tools — toggle AI tool integrations on/off |
| `h` | Help — keyboard shortcuts and info |
| `u` | Upgrade — enter API key |
| `q` | Quit |

## Commands

| Command | Description |
|---------|-------------|
| `tokara` | Start proxy + TUI dashboard |
| `tokara setup` | Run setup wizard |
| `tokara config` | View/edit configuration |
| `tokara test` | Run self-tests (routing, compaction, quality) |
| `tokara demo` | Demo mode with simulated traffic |
| `tokara upgrade` | Add API key for paid features |
| `tokara index <dir>` | Index codebase for RAG (paid) |
| `tokara help` | Show all commands |

## Supported Tools

| Tool | Detection | Configuration |
|------|-----------|---------------|
| Claude Code | Auto | Shell profile env vars |
| OpenAI Codex | Auto | Shell profile env vars |
| Aider | Auto | Shell profile env vars |
| Continue.dev | Auto | Shell profile env vars |
| Cursor | Auto | Manual (settings UI) |
| Windsurf | Auto | Manual (settings UI) |
| GitHub Copilot | Auto | Not supported |

When you enable a tool in the TUI, Tokara patches your shell profile (`~/.zshrc`, `~/.bashrc`, PowerShell profile) with SDK env vars so all new terminal sessions route through the proxy.

## Free vs Paid

| Feature | Free | Paid |
|---------|------|------|
| Compact compression | Local | Local |
| Smart hybrid compaction | Local | Local |
| Distill (query-aware) | — | Yes |
| Sift (semantic filtering) | — | Yes |
| Codebase RAG | — | Yes |

Free tier runs entirely on your machine — no account, no API calls.

Get your API key at [tokara.dev/dashboard](https://tokara.dev/dashboard).

## Configuration

Settings in `~/.tokara/config.toml`:

```toml
port = 18741
compaction_threshold = 0.80
precompute_threshold = 0.60
preserve_recent_turns = 4

# Uncomment for paid features
# api_key = "tk_live_..."
```

## Build from Source

```bash
go build -o tokara .
```

Requires Go 1.22+.

## License

MIT

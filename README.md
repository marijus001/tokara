# ▓ tokara

Context compression for LLMs — a local proxy with smart auto-compaction.

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
tokara              # first run: setup wizard → starts proxy
tokara status       # live stats dashboard
tokara stop         # stop the proxy
```

The setup wizard detects your AI tools (Claude Code, Cursor, Codex, OpenCode) and configures them to route through the proxy automatically.

## How It Works

```
AI Tool → localhost:18741 (tokara proxy) → LLM API
```

- **Under 60%** of context window: requests pass through untouched
- **At 60%**: background precomputation of compacted context begins
- **At 80%**: precomputed compaction is applied instantly — zero latency

Compression is structure-preserving: function signatures, types, imports, and recent tool outputs are never removed. Old conversational prose is summarized, code blocks are reduced to signatures (4:1 to 8:1 ratio).

## Commands

| Command | Description |
|---------|-------------|
| `tokara` | Start proxy (runs setup on first use) |
| `tokara setup` | Re-run setup wizard |
| `tokara status` | Live TUI dashboard |
| `tokara stop` | Stop the proxy |
| `tokara upgrade` | Add API key for paid features |
| `tokara index ./src` | Index codebase for RAG (paid) |
| `tokara help` | Show all commands |

## Free vs Paid

| Feature | Free | Paid |
|---------|------|------|
| Compact compression | ✓ Local | ✓ |
| Smart hybrid compaction | ✓ Local | ✓ |
| Distill (query-aware) | — | ✓ |
| Sift (semantic filtering) | — | ✓ |
| Codebase RAG | — | ✓ |

Free tier runs entirely on your machine — no account, no API calls.

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

# ▓ tokara

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
tokara run claude    # launch Claude Code through the proxy
tokara run aider     # launch Aider through the proxy
tokara               # start proxy + live TUI dashboard
tokara demo          # run with mock upstreams (no API key needed)
```

`tokara run <tool>` opens a new terminal with the tool pre-configured to route all LLM traffic through the proxy. The TUI dashboard stays in the original terminal showing live stats.

## How It Works

```
tokara run claude
  ├─ Terminal 1: TUI dashboard (live stats)
  └─ Terminal 2: claude (ANTHROPIC_BASE_URL → localhost:18741)
                    ↓
              tokara proxy → LLM API
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
| `t` | Tools — detect and launch AI tools |
| `h` | Help — keyboard shortcuts and info |
| `u` | Upgrade — enter API key |
| `q` | Quit |

Press `t` in the TUI to see detected tools. Select one and press enter to launch it in a new terminal routed through the proxy.

## Commands

| Command | Description |
|---------|-------------|
| `tokara run <tool>` | Launch a tool through the proxy |
| `tokara` | Start proxy + TUI dashboard |
| `tokara setup` | Run setup wizard |
| `tokara config` | View/edit configuration |
| `tokara test` | Run self-tests (routing, compaction, quality) |
| `tokara demo` | Demo mode with simulated traffic |
| `tokara upgrade` | Add API key for paid features |
| `tokara index <dir>` | Index codebase for RAG (paid) |
| `tokara help` | Show all commands |

## Supported Tools

| Tool | Launch | Detection |
|------|--------|-----------|
| Claude Code | `tokara run claude` | Auto |
| OpenAI Codex | `tokara run codex` | Auto |
| Aider | `tokara run aider` | Auto |
| Continue.dev | `tokara run continue` | Auto |
| Cursor | Manual (settings UI) | Auto |
| Windsurf | Manual (settings UI) | Auto |

## Enterprise

For on-premise deployment, bind the proxy to all interfaces:

```bash
tokara --bind 0.0.0.0
```

Add an auth token in `~/.tokara/config.toml`:

```toml
bind_address = "0.0.0.0"
auth_token = "your-secret-token"
```

Clients must include `Authorization: Bearer <token>` in requests. IT can route all LLM traffic through the proxy at the network level — no client-side configuration needed.

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

# Enterprise
# bind_address = "0.0.0.0"
# auth_token = "your-secret-token"

# Paid features
# api_key = "tk_live_..."
```

## Build from Source

```bash
go build -o tokara .
```

Requires Go 1.22+.

## License

MIT

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/marijus001/tokara/internal/api"
	"github.com/marijus001/tokara/internal/compactor"
	"github.com/marijus001/tokara/internal/config"
	"github.com/marijus001/tokara/internal/detect"
	tkctx "github.com/marijus001/tokara/internal/context"
	"github.com/marijus001/tokara/internal/proxy"
	"github.com/marijus001/tokara/internal/session"
	"github.com/marijus001/tokara/internal/setup"
	"github.com/marijus001/tokara/internal/stats"
)

const version = "0.1.7"

func main() {
	// Prevent charmbracelet/colorprofile from querying terminal (can hang when spawned from npx)
	if os.Getenv("COLORTERM") == "" {
		os.Setenv("COLORTERM", "truecolor")
	}

	port := flag.Int("port", 0, "override proxy port")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("tokara %s\n", version)
		os.Exit(0)
	}

	// Handle subcommands
	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "setup":
			setup.RunWizard(version)
			return
		case "upgrade":
			runUpgrade()
			return
		case "index":
			if len(args) < 2 {
				fmt.Println("  Usage: tokara index <directory>")
				os.Exit(1)
			}
			runIndex(args[1])
			return
		case "config":
			runConfig()
			return
		case "help", "--help", "-h":
			printHelp()
			return
		}
	}

	// Load config — create default on first run
	cfg, err := config.LoadFile(config.DefaultPath())
	if err != nil {
		// First run: create default config and start immediately.
		// The interactive wizard (tokara setup) can be run later.
		cfg = config.Defaults()
		if saveErr := cfg.SaveFile(config.DefaultPath()); saveErr == nil {
			fmt.Println()
			fmt.Printf("  \033[1;38;2;225;29;72m▓\033[0m \033[1mtokara\033[0m — first run\n")
			fmt.Println()
			fmt.Printf("  Created config at %s\n", config.DefaultPath())
			fmt.Println("  Run \033[1mtokara setup\033[0m to configure your AI tools")
			fmt.Println()
		}
	}
	cfg.ApplyEnv()

	if *port > 0 {
		cfg.Port = *port
	}

	// Run the proxy in the foreground
	runServer(cfg)
}

func runServer(cfg config.Config) {
	store := session.NewStore()

	// Periodic session cleanup
	go func() {
		for range time.Tick(10 * time.Minute) {
			removed := store.Cleanup(1 * time.Hour)
			if removed > 0 {
				log.Printf("[sessions] cleaned up %d stale sessions", removed)
			}
		}
	}()

	comp := compactor.New(compactor.Config{
		PrecomputeThreshold: cfg.PrecomputeThreshold,
		CompactThreshold:    cfg.CompactionThreshold,
		PreserveRecentTurns: cfg.PreserveRecentTurns,
		ModelContextWindow:  128000,
	}, store)

	// Context source (paid: cloud API, free: nil)
	var ctxSource tkctx.Source = &tkctx.NilSource{}
	if cfg.HasAPIKey() {
		client := api.NewClient(cfg.APIBase, cfg.APIKey)
		ctxSource = tkctx.NewCloudSource(client)
	}

	collector := stats.NewCollector(50)

	p := proxy.New(proxy.Options{
		Compactor:     comp,
		ContextSource: ctxSource,
	})

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mode := "free"
		if cfg.HasAPIKey() {
			mode = "paid"
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok", "version": version, "mode": mode, "sessions": store.Count(),
		})
	})

	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		snap := collector.BuildSnapshot(p.Stats.Requests.Load(), p.Stats.Compactions.Load(), p.Stats.TokensSaved.Load(), store.Count())
		data, _ := json.Marshal(snap)
		w.Write(data)
	})

	mux.Handle("/", p)

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	server := &http.Server{Addr: addr, Handler: mux}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		fmt.Println("\n  Stopping proxy...")
		server.Close()
	}()

	mode := "free"
	if cfg.HasAPIKey() {
		mode = "paid"
	}

	fmt.Println()
	fmt.Printf("  \033[1;38;2;225;29;72m▓\033[0m \033[1mtokara\033[0m v%s — proxy running\n", version)
	fmt.Println()
	fmt.Printf("  Mode:     %s\n", mode)
	fmt.Printf("  Proxy:    %s\n", addr)
	fmt.Printf("  Health:   %s/health\n", "http://"+addr)
	fmt.Println()
	fmt.Println("  Press \033[1mh\033[0m + enter for help, \033[1mq\033[0m + enter to quit")
	fmt.Println()

	// Interactive command handler
	go handleInteractive(config.DefaultPath(), store, collector, p, server)

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("  ✗ server error: %v", err)
	}
}

func runUpgrade() {
	fmt.Println()
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("  Enter your Tokara API key: ")
	key, _ := reader.ReadString('\n')
	key = strings.TrimSpace(key)

	if key == "" {
		fmt.Println("  ✗ No key provided")
		fmt.Println()
		return
	}
	if !strings.HasPrefix(key, "tk_live_") && !strings.HasPrefix(key, "tk_test_") {
		fmt.Println("  ✗ Invalid key — must start with tk_live_ or tk_test_")
		fmt.Println()
		return
	}

	if err := setup.SaveTokaraConfig(key, 18741); err != nil {
		fmt.Printf("  ✗ Failed to save: %v\n", err)
		fmt.Println()
		return
	}
	fmt.Println("  ✓ API key saved to ~/.tokara/config.toml")
	fmt.Println("  Restart the proxy to use paid features")
	fmt.Println()
}

func runIndex(dirPath string) {
	cfg, err := config.LoadFile(config.DefaultPath())
	if err != nil || !cfg.HasAPIKey() {
		fmt.Println()
		fmt.Println("  ✗ API key required for indexing. Run `tokara upgrade` first.")
		fmt.Println()
		os.Exit(1)
	}

	client := api.NewClient(cfg.APIBase, cfg.APIKey)
	if err := setup.RunIndex(client, dirPath, ""); err != nil {
		fmt.Printf("  ✗ %v\n", err)
		os.Exit(1)
	}
}

func runConfig() {
	configPath := config.DefaultPath()
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		fmt.Printf("  No config file found at %s\n", configPath)
		fmt.Println("  Run `tokara setup` to create one")
		fmt.Println()
		return
	}

	fmt.Println()
	fmt.Printf("  \033[1;38;2;225;29;72m▓\033[0m \033[1mtokara\033[0m config\n")
	fmt.Println()
	fmt.Printf("  File:       %s\n", configPath)
	fmt.Printf("  Port:       %d\n", cfg.Port)
	fmt.Printf("  Compact at: %.0f%% of context window\n", cfg.CompactionThreshold*100)
	fmt.Printf("  Precomp at: %.0f%% of context window\n", cfg.PrecomputeThreshold*100)
	fmt.Printf("  Keep turns: %d\n", cfg.PreserveRecentTurns)
	if cfg.HasAPIKey() {
		fmt.Printf("  API key:    %s...%s\n", cfg.APIKey[:10], cfg.APIKey[len(cfg.APIKey)-4:])
		fmt.Printf("  API base:   %s\n", cfg.APIBase)
		fmt.Printf("  Mode:       paid\n")
	} else {
		fmt.Printf("  Mode:       free (local only)\n")
	}
	fmt.Println()
	fmt.Printf("  Edit: %s\n", configPath)
	fmt.Println()
}

func printHelp() {
	fmt.Println()
	fmt.Printf("  \033[1;38;2;225;29;72m▓\033[0m \033[1mtokara\033[0m v%s — context compression for LLMs\n", version)
	fmt.Println()
	fmt.Println("  Commands:")
	fmt.Println("    tokara            Start the proxy (foreground, Ctrl+C to stop)")
	fmt.Println("    tokara setup      Run setup wizard again")
	fmt.Println("    tokara config     Show current configuration")
	fmt.Println("    tokara upgrade    Add API key for paid features")
	fmt.Println("    tokara index .    Index codebase for RAG (paid)")
	fmt.Println("    tokara help       Show this help")
	fmt.Println("    tokara --version  Print version")
	fmt.Println()
}

func handleInteractive(configPath string, store *session.Store, collector *stats.Collector, p *proxy.Proxy, server *http.Server) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, " ", 2)
		cmd := strings.ToLower(parts[0])
		args := ""
		if len(parts) > 1 {
			args = strings.TrimSpace(parts[1])
		}

		switch cmd {
		case "h", "help":
			printInteractiveHelp()
		case "s", "stats":
			printRunningStats(collector, p, store)
		case "c", "config":
			printRunningConfig(configPath)
		case "set":
			handleSet(args, configPath)
		case "u", "upgrade":
			fmt.Print("  Enter your Tokara API key: ")
			if scanner.Scan() {
				handleUpgrade(strings.TrimSpace(scanner.Text()), configPath)
			}
		case "t", "tools":
			printDetectedTools(configPath)
		case "setup":
			fmt.Println()
			fmt.Println("  To re-run setup, stop the proxy (q) and run: tokara setup")
			fmt.Println()
		case "l", "logs":
			printRecentLogs(collector, p, store)
		case "q", "quit":
			fmt.Println("\n  Stopping proxy...")
			server.Close()
			return
		case "":
			// ignore empty lines
		default:
			fmt.Printf("  Unknown command: %s (press h + enter for help)\n", cmd)
		}
	}
}

func printInteractiveHelp() {
	fmt.Println()
	fmt.Println("  Commands:")
	fmt.Println("    h              Show this help")
	fmt.Println("    s              Show proxy stats")
	fmt.Println("    c              Show running config")
	fmt.Println("    set <key> <v>  Change a config value")
	fmt.Println("    u              Add/update API key")
	fmt.Println("    t              Show detected AI tools")
	fmt.Println("    l              Show recent request logs")
	fmt.Println("    q              Stop proxy and exit")
	fmt.Println()
	fmt.Println("  Type 'set' to see available config keys")
	fmt.Println()
}

func printRunningStats(collector *stats.Collector, p *proxy.Proxy, store *session.Store) {
	snap := collector.BuildSnapshot(
		p.Stats.Requests.Load(),
		p.Stats.Compactions.Load(),
		p.Stats.TokensSaved.Load(),
		store.Count(),
	)
	fmt.Println()
	fmt.Printf("  \033[1;38;2;225;29;72m▓\033[0m \033[1mtokara\033[0m stats\n")
	fmt.Println()
	fmt.Printf("  Uptime:       %s\n", snap.Uptime)
	fmt.Printf("  Requests:     %d\n", snap.Requests)
	fmt.Printf("  Compactions:  %d\n", snap.Compactions)
	fmt.Printf("  Tokens saved: %d\n", snap.TokensSaved)
	fmt.Printf("  Sessions:     %d\n", snap.Sessions)
	if len(snap.RecentEvents) > 0 {
		fmt.Println()
		fmt.Println("  Recent:")
		for _, e := range snap.RecentEvents {
			fmt.Printf("    %s  %s/%s  %s  %dk→%dk", e.Timestamp, e.Provider, e.Model, e.Action, e.InputK, e.OutputK)
			if e.SavedPct > 0 {
				fmt.Printf("  (%d%% saved)", e.SavedPct)
			}
			fmt.Println()
		}
	}
	fmt.Println()
}

func printRunningConfig(configPath string) {
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		fmt.Printf("  ✗ Could not read config: %v\n", err)
		return
	}
	fmt.Println()
	fmt.Printf("  \033[1;38;2;225;29;72m▓\033[0m \033[1mtokara\033[0m config (%s)\n", configPath)
	fmt.Println()
	fmt.Printf("  Port:       %d\n", cfg.Port)
	fmt.Printf("  Compact at: %.0f%% of context window\n", cfg.CompactionThreshold*100)
	fmt.Printf("  Precomp at: %.0f%% of context window\n", cfg.PrecomputeThreshold*100)
	fmt.Printf("  Keep turns: %d\n", cfg.PreserveRecentTurns)
	if cfg.HasAPIKey() {
		fmt.Printf("  API key:    %s...%s\n", cfg.APIKey[:10], cfg.APIKey[len(cfg.APIKey)-4:])
		fmt.Printf("  Mode:       paid\n")
	} else {
		fmt.Printf("  Mode:       free (local only)\n")
	}
	fmt.Println()
}

func handleSet(args string, configPath string) {
	if args == "" {
		printSetHelp()
		return
	}

	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		printSetHelp()
		return
	}
	key := strings.ToLower(parts[0])
	val := strings.TrimSpace(parts[1])

	cfg, err := config.LoadFile(configPath)
	if err != nil {
		cfg = config.Defaults()
	}

	switch key {
	case "port":
		v, err := strconv.Atoi(val)
		if err != nil || v < 1 || v > 65535 {
			fmt.Println("  ✗ Invalid port (1-65535)")
			return
		}
		cfg.Port = v
	case "compact":
		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			fmt.Println("  ✗ Invalid threshold (use 0.0-1.0 or 0-100)")
			return
		}
		if v > 1 {
			v = v / 100
		}
		cfg.CompactionThreshold = v
	case "precompute":
		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			fmt.Println("  ✗ Invalid threshold (use 0.0-1.0 or 0-100)")
			return
		}
		if v > 1 {
			v = v / 100
		}
		cfg.PrecomputeThreshold = v
	case "turns":
		v, err := strconv.Atoi(val)
		if err != nil || v < 0 {
			fmt.Println("  ✗ Invalid turn count")
			return
		}
		cfg.PreserveRecentTurns = v
	case "apikey":
		cfg.APIKey = val
	default:
		fmt.Printf("  ✗ Unknown key: %s\n", key)
		printSetHelp()
		return
	}

	if err := cfg.SaveFile(configPath); err != nil {
		fmt.Printf("  ✗ Failed to save: %v\n", err)
		return
	}
	fmt.Printf("  ✓ %s updated (restart proxy to apply)\n", key)
}

func printSetHelp() {
	fmt.Println()
	fmt.Println("  Usage: set <key> <value>")
	fmt.Println()
	fmt.Println("  Keys:")
	fmt.Println("    port        Proxy port (default: 18741)")
	fmt.Println("    compact     Compaction threshold 0.0-1.0 (default: 0.80)")
	fmt.Println("    precompute  Precompute threshold 0.0-1.0 (default: 0.60)")
	fmt.Println("    turns       Preserve recent turns (default: 4)")
	fmt.Println("    apikey      Tokara API key")
	fmt.Println()
}

func handleUpgrade(key, configPath string) {
	if key == "" {
		fmt.Println("  ✗ No key provided")
		return
	}
	if !strings.HasPrefix(key, "tk_live_") && !strings.HasPrefix(key, "tk_test_") {
		fmt.Println("  ✗ Invalid key — must start with tk_live_ or tk_test_")
		return
	}
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		cfg = config.Defaults()
	}
	cfg.APIKey = key
	if err := cfg.SaveFile(configPath); err != nil {
		fmt.Printf("  ✗ Failed to save: %v\n", err)
		return
	}
	fmt.Println("  ✓ API key saved — restart proxy to enable paid features")
}

func printDetectedTools(configPath string) {
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		cfg = config.Defaults()
	}
	gatewayURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.Port)
	allTools := detect.AllTools(gatewayURL)
	detected := detect.DetectAll(allTools)

	fmt.Println()
	fmt.Printf("  \033[1;38;2;225;29;72m▓\033[0m \033[1mtokara\033[0m tools\n")
	fmt.Println()
	for _, t := range allTools {
		found := false
		for _, d := range detected {
			if d.ID == t.ID {
				found = true
				break
			}
		}
		if found {
			fmt.Printf("  \033[32m●\033[0m %s — %s\n", t.Name, t.Desc)
		} else {
			fmt.Printf("    %s — %s \033[2m(not found)\033[0m\n", t.Name, t.Desc)
		}
	}
	fmt.Println()
	if len(detected) == 0 {
		fmt.Println("  No AI tools detected.")
	} else {
		fmt.Printf("  %d tool(s) detected. Run 'tokara setup' to configure.\n", len(detected))
	}
	fmt.Println()
}

func printRecentLogs(collector *stats.Collector, p *proxy.Proxy, store *session.Store) {
	snap := collector.BuildSnapshot(
		p.Stats.Requests.Load(),
		p.Stats.Compactions.Load(),
		p.Stats.TokensSaved.Load(),
		store.Count(),
	)
	fmt.Println()
	if len(snap.RecentEvents) == 0 {
		fmt.Println("  No recent events")
	} else {
		fmt.Printf("  \033[1;38;2;225;29;72m▓\033[0m \033[1mtokara\033[0m logs (last %d)\n", len(snap.RecentEvents))
		fmt.Println()
		for _, e := range snap.RecentEvents {
			fmt.Printf("  %s  %-10s %-20s %s  %dk→%dk", e.Timestamp, e.Provider, e.Model, e.Action, e.InputK, e.OutputK)
			if e.SavedPct > 0 {
				fmt.Printf("  (%d%% saved)", e.SavedPct)
			}
			fmt.Println()
		}
	}
	fmt.Println()
}

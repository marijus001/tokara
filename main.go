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
	"strings"
	"syscall"
	"time"

	"github.com/marijus001/tokara/internal/api"
	"github.com/marijus001/tokara/internal/compactor"
	"github.com/marijus001/tokara/internal/config"
	tkctx "github.com/marijus001/tokara/internal/context"
	"github.com/marijus001/tokara/internal/proxy"
	"github.com/marijus001/tokara/internal/session"
	"github.com/marijus001/tokara/internal/setup"
	"github.com/marijus001/tokara/internal/stats"
)

const version = "0.1.3"

func main() {
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

	// First run: setup wizard if no config exists
	cfg, err := config.LoadFile(config.DefaultPath())
	if err != nil {
		setup.RunWizard(version)
		cfg, _ = config.LoadFile(config.DefaultPath())
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
	fmt.Println("  Ctrl+C to stop")
	fmt.Println()

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

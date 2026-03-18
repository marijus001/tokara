package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/marijus001/tokara/internal/api"
	"github.com/marijus001/tokara/internal/cli"
	"github.com/marijus001/tokara/internal/compactor"
	"github.com/marijus001/tokara/internal/config"
	tkctx "github.com/marijus001/tokara/internal/context"
	"github.com/marijus001/tokara/internal/daemon"
	"github.com/marijus001/tokara/internal/proxy"
	"github.com/marijus001/tokara/internal/session"
	"github.com/marijus001/tokara/internal/stats"
)

const version = "0.1.0"

func main() {
	// Check for --daemon-child flag (started by daemon.Start)
	daemonChild := flag.Bool("daemon-child", false, "run as daemon child process")
	port := flag.Int("port", 0, "override proxy port")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("tokara %s\n", version)
		os.Exit(0)
	}

	// If not daemon-child, route to CLI commands
	if !*daemonChild {
		cli.Run(version, flag.Args())
		return
	}

	// ── Daemon child mode: run the proxy server ──
	runServer(*port)
}

func runServer(portOverride int) {
	cfg, err := config.LoadFile(config.DefaultPath())
	if err != nil && !os.IsNotExist(err) {
		log.Fatalf("config error: %v", err)
	}
	cfg.ApplyEnv()

	if portOverride > 0 {
		cfg.Port = portOverride
	}

	// Write PID file
	daemon.WritePid(os.Getpid())

	// Session store + compactor
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
		log.Printf("context source: cloud (%s)", cfg.APIBase)
	} else {
		log.Println("context source: none (free tier)")
	}

	// Stats collector
	collector := stats.NewCollector(50)

	p := proxy.New(proxy.Options{
		Compactor:     comp,
		ContextSource: ctxSource,
	})

	mux := http.NewServeMux()

	// Health endpoint
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

	// Stats endpoint (for TUI)
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		snap := collector.BuildSnapshot(p.Stats.Requests.Load(), p.Stats.Compactions.Load(), p.Stats.TokensSaved.Load(), store.Count())
		data, _ := json.Marshal(snap)
		w.Write(data)
	})

	// Proxy handler
	mux.Handle("/", p)

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	server := &http.Server{Addr: addr, Handler: mux}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		daemon.RemovePid()
		server.Close()
	}()

	mode := "free"
	if cfg.HasAPIKey() {
		mode = "paid"
	}
	log.Printf("tokara proxy v%s listening on %s (mode: %s, pid: %d)", version, addr, mode, os.Getpid())

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		daemon.RemovePid()
		log.Fatalf("server error: %v", err)
	}
}

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"proxymaxxing/the_bouncer"
	"proxymaxxing/the_conduit"
	"proxymaxxing/the_oracle"
	"proxymaxxing/the_stage"
)

func main() {
	configPath := flag.String("config", "", "Path to config.yaml")
	flag.Parse()

	if *configPath == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			*configPath = "config.yaml"
		} else {
			fmt.Println("Error: config.yaml not found in current directory.")
			fmt.Println("Please provide a config file using: proxymaxxing --config <path_to_config>")
			os.Exit(1)
		}
	}

	cfg, err := the_oracle.Read(*configPath)
	if err != nil {
		log.Fatalf("The Oracle failed to read the runes: %v", err)
	}

	// Pre-hydrate before launching UI to avoid complex async loading UI state
	the_oracle.Hydrate(cfg, *configPath)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		// Trigger cleanup if killed aggressively (e.g. SIGTERM)
		the_conduit.Teardown(cfg.VPNProfileName)
		os.Exit(0)
	}()

	conduitStatus, err := the_conduit.Setup(cfg)
	if err != nil {
		log.Printf("The Conduit failed to route: %v", err)
	}

	logChan := make(chan the_bouncer.LogEvent, 100)

	proxy := the_bouncer.Setup(cfg, logChan)

	// Start proxy
	go func() {
		addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)

		corsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "OPTIONS" {
				origin := r.Header.Get("Origin")
				if origin == "" {
					origin = "*"
				}
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin, traceparent")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			proxy.ServeHTTP(w, r)
		})

		if err := http.ListenAndServe(addr, corsHandler); err != nil {
			log.Fatalf("The Bouncer got knocked out: %v", err)
		}
	}()

	// Start TUI
	p := tea.NewProgram(the_stage.InitialModel(cfg, *configPath, logChan, conduitStatus), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("The Stage collapsed: %v", err)
	}
	
	// Graceful shutdown after TUI exits (e.g. user presses 'q')
	the_conduit.Teardown(cfg.VPNProfileName)
}

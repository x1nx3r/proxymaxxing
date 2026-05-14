package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"proxymaxxing/the_bouncer"
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

	logChan := make(chan the_bouncer.LogEvent, 100)

	proxy := the_bouncer.Setup(cfg, logChan)

	// Start proxy
	go func() {
		addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
		if err := http.ListenAndServe(addr, proxy); err != nil {
			log.Fatalf("The Bouncer got knocked out: %v", err)
		}
	}()

	// Start TUI
	p := tea.NewProgram(the_stage.InitialModel(cfg, *configPath, logChan), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("The Stage collapsed: %v", err)
	}
}

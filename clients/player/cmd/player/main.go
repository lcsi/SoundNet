package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"player/internal/player"
)

func main() {
	// ---------------------------------------------------------------
	// Command-line flags
	// ---------------------------------------------------------------
	serverAddr := flag.String("server", "ws://localhost:8080",
		"Backend server WebSocket address (e.g. ws://192.168.1.100:8080)")
	playerID := flag.String("player-id", "",
		"Unique player identifier (auto-generated if empty)")
	playerName := flag.String("name", "",
		"Display name for this player (defaults to player-id)")
	channel := flag.String("channel", "",
		"Channel to join immediately after registration")
	verbose := flag.Bool("v", false, "Enable verbose logging")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Player — Remote music playback client\n\n")
		fmt.Fprintf(os.Stderr, "Connects to a music-player backend server via WebSocket and\n")
		fmt.Fprintf(os.Stderr, "plays audio from remote commands.\n\n")
		fmt.Fprintf(os.Stderr, "Supports: Android, Linux, Windows\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s [flags]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s --server ws://192.168.1.100:8080 --channel my-music\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --server ws://192.168.1.100:8080 --name LivingRoom --channel main\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nThe server address is the same as the backend API server.\n")
		fmt.Fprintf(os.Stderr, "Player ID persists across restarts; set explicitly or let it auto-generate.\n")
	}

	flag.Parse()

	if !*verbose {
		// Suppress log output unless verbose
		log.SetFlags(0)
	}

	// Validate server address
	if *serverAddr == "" {
		log.Fatal("server address is required")
	}

	log.Printf("=== Player Starting ===")
	log.Printf(" Server: %s", *serverAddr)
	if *channel != "" {
		log.Printf(" Channel: %s", *channel)
	}

	// ---------------------------------------------------------------
	// Create and run player
	// ---------------------------------------------------------------
	p, err := player.NewPlayer(*serverAddr, *playerID, *playerName, *channel)
	if err != nil {
		log.Fatalf("Failed to create player: %v", err)
	}

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("\nShutting down...")
		p.Stop()
	}()

	if err := p.Run(); err != nil {
		log.Fatalf("Player error: %v", err)
	}

	log.Println("Player stopped.")
}

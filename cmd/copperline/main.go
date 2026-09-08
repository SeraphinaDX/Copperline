package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"copperline/internal/config"
	"copperline/internal/irc"
	"copperline/internal/relay"
	"copperline/internal/tui"
)

const version = "0.2.1"

func main() {
	configPath := flag.String("config", config.DefaultPath(), "path to Copperline TOML configuration")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("Copperline", version)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Fatalf("configuration not found: %s\nCopy config.example.toml there or run with -config=/path/to/config.toml", *configPath)
		}
		log.Fatalf("load configuration: %v", err)
	}

	switch cfg.Relay.ModeValue() {
	case "server":
		r, err := relay.NewServer(cfg)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Copperline relay listening on %s", cfg.Relay.Listen)
		log.Printf("Copperline relay host key fingerprint: %s", r.HostKeyFingerprint())
		log.Printf("Copperline relay authorized keys: %s", config.ExpandPath(cfg.Relay.AuthorizedKeys))
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := r.Run(ctx); err != nil {
			log.Fatal(err)
		}

	case "client":
		backend, err := relay.NewClient(cfg)
		if err != nil {
			log.Fatal(err)
		}
		app := tui.NewWithBackend(cfg, backend)
		app.SetRelayReconnectFactory(func() (irc.Backend, error) {
			return relay.NewClient(cfg)
		})
		if err := app.Run(); err != nil {
			log.Fatal(err)
		}

	default:
		app := tui.New(cfg)
		if err := app.Run(); err != nil {
			log.Fatal(err)
		}
	}
}

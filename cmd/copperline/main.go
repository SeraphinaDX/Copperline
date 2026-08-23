package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"copperline/internal/config"
	"copperline/internal/tui"
)

const version = "0.1.0"

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

	app := tui.New(cfg)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

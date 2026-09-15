package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/legion/display/internal/autobrightness"
	"github.com/legion/display/internal/brightness"
	"github.com/legion/display/internal/config"
	dbussvc "github.com/legion/display/internal/dbus"
	"github.com/legion/display/internal/ddcutil"
)

func main() {
	verbose := flag.Bool("v", false, "verbose logging")
	ddcutilPath := flag.String("ddcutil-path", "ddcutil", "path to ddcutil binary")
	timeout := flag.Duration("timeout", 5*time.Second, "per-command ddcutil timeout")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("display-brightnessd: ")

	cfg, err := config.Load()
	if err != nil {
		log.Printf("config load: %v (using defaults)", err)
		cfg = config.Default()
	}

	conn, err := dbus.SessionBus()
	if err != nil {
		log.Fatalf("session bus: %v", err)
	}

	client := ddcutil.NewClient(*ddcutilPath, *timeout, *verbose)
	controller := brightness.NewController(client, *verbose)
	auto := autobrightness.NewManager(controller, cfg)

	service := dbussvc.NewService(conn, controller, auto)
	if err := service.Export(); err != nil {
		log.Fatalf("export dbus: %v", err)
	}
	if err := dbussvc.AcquireName(conn); err != nil {
		log.Fatalf("acquire name: %v", err)
	}

	log.Printf("ready on %s (auto=%v)", dbussvc.BusName, cfg.Auto)

	go func() {
		ctx := context.Background()
		controller.DiscoverAtStartup(ctx)
		if client.ProbeI2C0Noise(ctx) {
			log.Printf("/dev/i2c-0 not accessible (unused bus) — harmless, see README")
		}
		service.EmitCurrentBrightness(ctx)
		if cfg.Auto {
			auto.Start()
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Printf("shutting down")
	auto.Stop()
}

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ermos/docker-redis-backup/internal/backup"
	"github.com/ermos/docker-redis-backup/internal/config"
	"github.com/ermos/docker-redis-backup/internal/storage"
	"github.com/robfig/cron/v3"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Redis Backup Service...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded:")
	log.Printf("  Redis: %s:%s", cfg.RedisHost, cfg.RedisPort)
	log.Printf("  Backup schedule: %s", cfg.BackupCron)
	log.Printf("  Storage type: %s", cfg.StorageType)
	log.Printf("  Retention count: %d", cfg.RetentionCount)

	// Initialize storage
	store, err := storage.New(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	log.Printf("Storage initialized: %s", store.Type())

	// Initialize backup manager
	backupManager, err := backup.New(cfg, store)
	if err != nil {
		log.Fatalf("Failed to initialize backup manager: %v", err)
	}
	defer backupManager.Close()
	log.Println("Backup manager initialized, connected to Redis")

	// Run backup on start if configured
	if cfg.BackupOnStart {
		log.Println("Running initial backup on startup...")
		if err := backupManager.Run(context.Background()); err != nil {
			log.Printf("Initial backup failed: %v", err)
		}
	}

	// Setup cron scheduler
	c := cron.New(cron.WithParser(cron.NewParser(
		cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)))

	// Add backup job
	entryID, err := c.AddFunc(cfg.BackupCron, func() {
		log.Println("Cron triggered backup job")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		if err := backupManager.Run(ctx); err != nil {
			log.Printf("Backup failed: %v", err)
		}
	})
	if err != nil {
		log.Fatalf("Failed to add cron job: %v", err)
	}
	log.Printf("Cron job registered with ID: %d", entryID)

	// Start cron scheduler
	c.Start()
	log.Println("Cron scheduler started, waiting for scheduled jobs...")

	// Print next scheduled run time
	entries := c.Entries()
	if len(entries) > 0 {
		log.Printf("Next backup scheduled at: %s", entries[0].Next.Format("2006-01-02 15:04:05"))
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Printf("Received signal %s, shutting down...", sig)

	// Stop cron scheduler
	ctx := c.Stop()
	<-ctx.Done()

	log.Println("Shutdown complete")
}

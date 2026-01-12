package backup

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ermos/docker-redis-backup/internal/config"
	"github.com/ermos/docker-redis-backup/internal/storage"
	"github.com/redis/go-redis/v9"
)

// Manager handles Redis backup operations
type Manager struct {
	cfg     *config.Config
	redis   *redis.Client
	storage storage.Storage
}

// New creates a new backup manager with retry logic for Redis connection
func New(cfg *config.Config, store storage.Storage) (*Manager, error) {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	// Retry connection with exponential backoff
	maxRetries := 10
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := redisClient.Ping(ctx).Err()
		cancel()

		if err == nil {
			return &Manager{
				cfg:     cfg,
				redis:   redisClient,
				storage: store,
			}, nil
		}

		lastErr = err
		waitTime := time.Duration(i+1) * 2 * time.Second
		if waitTime > 30*time.Second {
			waitTime = 30 * time.Second
		}
		log.Printf("Failed to connect to Redis (attempt %d/%d): %v. Retrying in %s...", i+1, maxRetries, err, waitTime)
		time.Sleep(waitTime)
	}

	_ = redisClient.Close()
	return nil, fmt.Errorf("failed to connect to Redis after %d attempts: %w", maxRetries, lastErr)
}

// Run executes a backup operation
func (m *Manager) Run(ctx context.Context) error {
	log.Println("Starting backup process...")

	// Step 1: Trigger BGSAVE
	if err := m.triggerBGSAVE(ctx); err != nil {
		return fmt.Errorf("failed to trigger BGSAVE: %w", err)
	}

	// Step 2: Wait for BGSAVE to complete
	if err := m.waitForBGSAVE(ctx); err != nil {
		return fmt.Errorf("failed waiting for BGSAVE: %w", err)
	}

	// Step 3: Generate backup filename with timestamp
	backupName := m.generateBackupName()

	// Step 4: Upload RDB file to storage
	rdbPath := filepath.Join(m.cfg.RedisDataPath, "dump.rdb")
	if err := m.storage.Upload(ctx, rdbPath, backupName); err != nil {
		return fmt.Errorf("failed to upload backup: %w", err)
	}

	log.Printf("Backup completed successfully: %s (storage: %s)", backupName, m.storage.Type())

	// Step 5: Apply retention policy
	if m.cfg.RetentionCount > 0 {
		if err := m.applyRetention(ctx); err != nil {
			log.Printf("Warning: failed to apply retention policy: %v", err)
		}
	}

	return nil
}

// triggerBGSAVE initiates a background save in Redis
func (m *Manager) triggerBGSAVE(ctx context.Context) error {
	log.Println("Triggering BGSAVE...")

	// Check if BGSAVE is already in progress
	info, err := m.redis.Info(ctx, "persistence").Result()
	if err != nil {
		return fmt.Errorf("failed to get persistence info: %w", err)
	}

	if containsBGSAVEInProgress(info) {
		log.Println("BGSAVE already in progress, waiting...")
		return nil
	}

	// Trigger BGSAVE
	if err := m.redis.BgSave(ctx).Err(); err != nil {
		return fmt.Errorf("BGSAVE command failed: %w", err)
	}

	return nil
}

// waitForBGSAVE waits for the background save to complete
func (m *Manager) waitForBGSAVE(ctx context.Context) error {
	log.Println("Waiting for BGSAVE to complete...")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			info, err := m.redis.Info(ctx, "persistence").Result()
			if err != nil {
				return fmt.Errorf("failed to get persistence info: %w", err)
			}

			if !containsBGSAVEInProgress(info) {
				log.Println("BGSAVE completed")
				return nil
			}
		}
	}
}

// generateBackupName creates a unique backup filename
func (m *Manager) generateBackupName() string {
	timestamp := time.Now().UTC().Format("2006-01-02_15-04-05")
	return fmt.Sprintf("redis-backup_%s.rdb", timestamp)
}

// applyRetention removes old backups beyond retention count
func (m *Manager) applyRetention(ctx context.Context) error {
	log.Printf("Applying retention policy (keeping %d backups)...", m.cfg.RetentionCount)

	backups, err := m.storage.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list backups: %w", err)
	}

	if len(backups) <= m.cfg.RetentionCount {
		log.Printf("Current backup count (%d) within retention limit", len(backups))
		return nil
	}

	// Delete oldest backups (list is sorted oldest first)
	toDelete := len(backups) - m.cfg.RetentionCount
	for i := 0; i < toDelete; i++ {
		log.Printf("Deleting old backup: %s", backups[i])
		if err := m.storage.Delete(ctx, backups[i]); err != nil {
			log.Printf("Warning: failed to delete %s: %v", backups[i], err)
		}
	}

	log.Printf("Retention policy applied, deleted %d old backup(s)", toDelete)
	return nil
}

// Close closes the Redis connection
func (m *Manager) Close() error {
	return m.redis.Close()
}

// containsBGSAVEInProgress checks if a BGSAVE is currently running
func containsBGSAVEInProgress(info string) bool {
	// Redis returns rdb_bgsave_in_progress:1 when BGSAVE is running
	return containsString(info, "rdb_bgsave_in_progress:1")
}

// containsString checks if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// CheckRDBFile verifies that the RDB file exists
func (m *Manager) CheckRDBFile() error {
	rdbPath := filepath.Join(m.cfg.RedisDataPath, "dump.rdb")
	if _, err := os.Stat(rdbPath); os.IsNotExist(err) {
		return fmt.Errorf("RDB file not found at %s - ensure REDIS_DATA_PATH is correctly configured", rdbPath)
	}
	return nil
}

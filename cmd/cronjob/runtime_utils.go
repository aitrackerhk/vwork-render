package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
)

func loadDotEnv() {
	// Priority:
	// 1) ENV_FILE (absolute or relative) if provided
	// 2) .env in current working directory
	// 3) .env next to the executable (useful for Windows Task Scheduler / service)
	if p := os.Getenv("ENV_FILE"); p != "" {
		if err := godotenv.Load(p); err != nil {
			log.Printf("⚠️  Warning: failed to load ENV_FILE=%s: %v (using environment variables)", p, err)
		}
		return
	}

	if err := godotenv.Load(); err == nil {
		return
	}

	// Fallback: next to executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		exeEnv := filepath.Join(exeDir, ".env")
		if err2 := godotenv.Load(exeEnv); err2 == nil {
			log.Printf("✅ Loaded .env from executable directory: %s", exeEnv)
			return
		}
	}

	log.Println("⚠️  Warning: .env file not found, using environment variables")
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var out int
	if _, err := fmt.Sscanf(v, "%d", &out); err != nil {
		return def
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}



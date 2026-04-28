package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"nwork/config"
	"nwork/internal/database"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	// 加載 .env 文件
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  Warning: .env file not found, using environment variables")
	}

	cfg := config.Load()
	if err := database.Connect(cfg); err != nil {
		log.Fatalf("failed to connect DB: %v", err)
	}
	// 避免在終端輸出洩漏 DB 密碼：只輸出安全摘要
	dsn := cfg.Database.DSN()
	masked := dsn
	if i := strings.Index(strings.ToLower(masked), "password="); i >= 0 {
		// password=xxx 直到空格或結尾
		start := i + len("password=")
		end := start
		for end < len(masked) && masked[end] != ' ' {
			end++
		}
		masked = masked[:start] + "******" + masked[end:]
	}
	fmt.Println("Using DSN:", masked)

	// 支援：指定要套用的 migration 檔名（例如 102_create_job_applicants.sql）
	// 用法：
	//   go run cmd/apply_migrations/main.go 102_create_job_applicants.sql
	args := os.Args[1:]
	files := []string{}
	if len(args) > 0 {
		files = args
	} else {
		// 預設：套用 migrations 目錄內所有 .sql（以檔名排序）
		// 注意：001_initial_schema.sql 與 002_cms_modules_schema.sql 是「重建資料庫」用的大型 schema，
		// 現有 DB 通常已建好，直接重跑可能會因 CREATE TABLE (無 IF NOT EXISTS) 而失敗。
		// 因此預設跳過 001/002；如需重建 DB，請明確指定檔名作為參數來執行。
		entries, err := ioutil.ReadDir("migrations")
		if err != nil {
			log.Fatalf("read migrations dir: %v", err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(strings.ToLower(name), ".sql") {
				continue
			}
			if strings.HasPrefix(name, "001_") || strings.HasPrefix(name, "002_") {
				continue
			}
			if name == "rebuild_db.sql" || name == "__inspect_columns.sql" {
				continue
			}
			files = append(files, name)
		}
		sort.Strings(files)
	}

	// Multi-pass: continue on error and retry failed ones, until no progress.
	pending := make([]string, len(files))
	copy(pending, files)
	pass := 0
	for len(pending) > 0 {
		pass++
		var failed []string
		var lastErr error
		for _, f := range pending {
			path := filepath.Join("migrations", f)
			fmt.Printf("[pass %d] Applying: %s\n", pass, path)
			sqlBytes, err := ioutil.ReadFile(path)
			if err != nil {
				log.Fatalf("read %s: %v", path, err)
			}
			if err := database.DB.Exec(string(sqlBytes)).Error; err != nil {
				fmt.Printf("[pass %d] FAILED: %s -> %v\n", pass, f, err)
				failed = append(failed, f)
				lastErr = err
			}
		}
		if len(failed) == len(pending) {
			fmt.Printf("No progress in pass %d; %d migrations still failing. Last error: %v\n", pass, len(failed), lastErr)
			for _, f := range failed {
				fmt.Println("  - still failing:", f)
			}
			os.Exit(1)
		}
		pending = failed
	}

	fmt.Println("All migrations applied.")
}

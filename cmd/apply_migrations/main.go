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
			// Split on `;` newline and exec each stmt; swallow "already exists" / duplicate errors.
			stmts := splitSQL(string(sqlBytes))
			var fileErr error
			ok, skip, dup := 0, 0, 0
			for _, stmt := range stmts {
				s := strings.TrimSpace(stmt)
				if s == "" {
					continue
				}
				if execErr := database.DB.Exec(s).Error; execErr != nil {
					msg := strings.ToLower(execErr.Error())
					if strings.Contains(msg, "already exists") || strings.Contains(msg, "duplicate") {
						dup++
						continue
					}
					// real error — record but continue (other statements may still apply).
					fileErr = execErr
					skip++
					fmt.Printf("[pass %d]   ! %s -> %v\n", pass, firstLine(s), execErr)
				} else {
					ok++
				}
			}
			fmt.Printf("[pass %d]   %s: ok=%d dup=%d err=%d\n", pass, f, ok, dup, skip)
			if fileErr != nil && skip > 0 {
				failed = append(failed, f)
				lastErr = fileErr
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

// splitSQL splits a SQL script into statements by `;`, but respects
// dollar-quoted bodies ($$ ... $$) and single-quoted strings, and ignores
// `;` inside line comments (--) and block comments (/* */).
func splitSQL(sql string) []string {
	var out []string
	var cur strings.Builder
	i, n := 0, len(sql)
	inSingle := false
	inDollar := false
	dollarTag := ""
	for i < n {
		c := sql[i]
		// line comment
		if !inSingle && !inDollar && c == '-' && i+1 < n && sql[i+1] == '-' {
			for i < n && sql[i] != '\n' {
				cur.WriteByte(sql[i])
				i++
			}
			continue
		}
		// block comment
		if !inSingle && !inDollar && c == '/' && i+1 < n && sql[i+1] == '*' {
			cur.WriteByte(sql[i])
			cur.WriteByte(sql[i+1])
			i += 2
			for i+1 < n && !(sql[i] == '*' && sql[i+1] == '/') {
				cur.WriteByte(sql[i])
				i++
			}
			if i+1 < n {
				cur.WriteByte(sql[i])
				cur.WriteByte(sql[i+1])
				i += 2
			}
			continue
		}
		// dollar quote start/end
		if !inSingle && c == '$' {
			j := i + 1
			for j < n && (sql[j] == '_' || (sql[j] >= 'a' && sql[j] <= 'z') || (sql[j] >= 'A' && sql[j] <= 'Z') || (sql[j] >= '0' && sql[j] <= '9')) {
				j++
			}
			if j < n && sql[j] == '$' {
				tag := sql[i : j+1]
				if inDollar && tag == dollarTag {
					cur.WriteString(tag)
					i = j + 1
					inDollar = false
					dollarTag = ""
					continue
				}
				if !inDollar {
					cur.WriteString(tag)
					i = j + 1
					inDollar = true
					dollarTag = tag
					continue
				}
			}
		}
		if inDollar {
			cur.WriteByte(c)
			i++
			continue
		}
		// single-quote string
		if c == '\'' {
			cur.WriteByte(c)
			i++
			if inSingle && i < n && sql[i] == '\'' {
				cur.WriteByte(sql[i])
				i++
				continue
			}
			inSingle = !inSingle
			continue
		}
		if !inSingle && c == ';' {
			out = append(out, cur.String())
			cur.Reset()
			i++
			continue
		}
		cur.WriteByte(c)
		i++
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, cur.String())
	}
	return out
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			s = s[:i]
			break
		}
	}
	if len(s) > 80 {
		s = s[:80] + "..."
	}
	return strings.TrimSpace(s)
}

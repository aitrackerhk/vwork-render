package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"os"
	"sync"
	"time"

	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/models"

	"gorm.io/gorm"
)

// startEmailQueueWorkers runs the old email_worker logic inside cronjob.
// It polls email_jobs every EMAIL_WORKER_POLL_MS (default: 10000ms) with concurrency workers.
func startEmailQueueWorkers(ctx context.Context, cfg *config.Config) {
	email.SetConfig(cfg)

	// Fail fast if SMTP is not configured (avoid burning job retries).
	// You can override this in dev by setting EMAIL_WORKER_ALLOW_NO_SMTP=true.
	allowNoSMTP := os.Getenv("EMAIL_WORKER_ALLOW_NO_SMTP")
	if err := email.ValidateSMTPConfig(); err != nil {
		if allowNoSMTP == "true" {
			log.Printf("⚠️  SMTP not configured (%v). Continuing because EMAIL_WORKER_ALLOW_NO_SMTP=true", err)
		} else {
			log.Fatalf("❌ SMTP not configured (%v). Set required env vars or set EMAIL_WORKER_ALLOW_NO_SMTP=true to bypass.", err)
		}
	}

	workerID := os.Getenv("EMAIL_WORKER_ID")
	if workerID == "" {
		workerID = fmt.Sprintf("cronjob-email-worker-%d", time.Now().Unix())
	}

	concurrency := getEnvInt("EMAIL_WORKER_CONCURRENCY", 5)
	// Requested default: every 10 seconds
	pollMS := getEnvInt("EMAIL_WORKER_POLL_MS", 10000)
	lockStaleSeconds := getEnvInt("EMAIL_WORKER_LOCK_STALE_SECONDS", 600)

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			emailLoop(ctx, workerID, n, time.Duration(pollMS)*time.Millisecond, time.Duration(lockStaleSeconds)*time.Second)
		}(i)
	}

	<-ctx.Done()
	log.Println("🛑 Email queue worker shutting down (cronjob)...")
	wg.Wait()
	log.Println("✅ Email queue worker stopped (cronjob)")
}

func emailLoop(ctx context.Context, workerID string, workerNum int, pollInterval time.Duration, lockStale time.Duration) {
	logger := log.New(os.Stdout, fmt.Sprintf("[cronjob-email %s #%d] ", workerID, workerNum), log.LstdFlags)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, ok, err := fetchAndLockOneEmailJob(workerID, lockStale)
		if err != nil {
			logger.Printf("fetch error: %v", err)
			sleep(ctx, pollInterval)
			continue
		}
		if !ok {
			sleep(ctx, pollInterval)
			continue
		}

		attachments := []email.Attachment{}
		if job.Attachments != nil {
			for _, a := range job.Attachments {
				attachments = append(attachments, email.Attachment{
					Name:        a.Name,
					ContentType: a.ContentType,
					Data:        a.Data,
				})
			}
		}

		err = email.SendSMTP(email.Message{
			ToEmail:     job.ToEmail,
			Subject:     job.Subject,
			TextBody:    job.BodyText,
			HTMLBody:    job.BodyHTML,
			FromEmail:   "", // use config default
			FromName:    "",
			Attachments: attachments,
		})
		if err == nil {
			if err2 := markEmailJobSent(job.ID); err2 != nil {
				logger.Printf("mark sent error: job_id=%d err=%v", job.ID, err2)
			} else {
				logger.Printf("sent: job_id=%d kind=%s to=%s", job.ID, job.Kind, job.ToEmail)
			}
			continue
		}

		if err2 := markEmailJobFailed(job, err); err2 != nil {
			logger.Printf("mark failed error: job_id=%d send_err=%v mark_err=%v", job.ID, err, err2)
		} else {
			logger.Printf("send failed: job_id=%d kind=%s to=%s err=%v", job.ID, job.Kind, job.ToEmail, err)
		}
	}
}

func fetchAndLockOneEmailJob(workerID string, lockStale time.Duration) (models.EmailJob, bool, error) {
	var job models.EmailJob

	q := `
WITH picked AS (
  SELECT id
  FROM email_jobs
  WHERE (
      status = 'queued' AND run_at <= NOW()
  ) OR (
      status = 'sending' AND locked_at IS NOT NULL AND locked_at < (NOW() - (? * INTERVAL '1 second'))
  )
  ORDER BY run_at ASC, id ASC
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE email_jobs
SET status = 'sending',
    locked_at = NOW(),
    locked_by = ?,
    updated_at = NOW()
WHERE id IN (SELECT id FROM picked)
RETURNING *;
`

	res := database.DB.Raw(q, int(lockStale.Seconds()), workerID).Scan(&job)
	if res.Error != nil {
		return models.EmailJob{}, false, res.Error
	}
	if res.RowsAffected == 0 {
		return models.EmailJob{}, false, nil
	}
	return job, true, nil
}

func markEmailJobSent(id uint64) error {
	return database.DB.Exec(`
UPDATE email_jobs
SET status='sent',
    attachments=NULL,
    locked_at=NULL,
    locked_by=NULL,
    last_error=NULL,
    updated_at=NOW()
WHERE id = ?;
`, id).Error
}

func markEmailJobFailed(job models.EmailJob, sendErr error) error {
	nextAttempts := job.Attempts + 1
	status := "queued"
	if nextAttempts >= 5 {
		status = "dead"
	}

	delay := emailBackoff(nextAttempts)
	nextRunAt := time.Now().Add(delay)

	errStr := truncate(sendErr.Error(), 2000)

	return database.DB.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(`
UPDATE email_jobs
SET status = ?,
    attempts = ?,
    run_at = ?,
    locked_at = NULL,
    locked_by = NULL,
    last_error = ?,
    updated_at = NOW()
WHERE id = ?;
`, status, nextAttempts, nextRunAt, errStr, job.ID).Error
	})
}

func emailBackoff(attempt int) time.Duration {
	// 1->5s, 2->10s, 3->20s, ... capped at 300s (+ jitter)
	base := 5.0 * math.Pow(2, float64(max(0, attempt-1)))
	if base > 300 {
		base = 300
	}
	jitter := rand.IntN(4) // 0-3s
	return time.Duration(base+float64(jitter)) * time.Second
}

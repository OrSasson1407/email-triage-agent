package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"email-triage-agent/ai"
	"email-triage-agent/config"
	"email-triage-agent/db"
	"email-triage-agent/gmail"
	"email-triage-agent/notify"
	"email-triage-agent/store"
)

type Scheduler struct {
	cfg         config.Config
	logger      *zap.Logger
	cron        *cron.Cron
	classifier  *ai.Classifier
	gmailClient *gmail.Client
	redisStore  store.Store
	database    *db.DB
	slack       *notify.Slack
	emailDigest *notify.EmailDigest
	limiter     *rate.Limiter

	// metrics
	mu        sync.Mutex
	processed int
	failed    int
	lastRun   time.Time
}

func New(cfg config.Config, logger *zap.Logger) *Scheduler {
	gmailClient, err := gmail.NewClient(cfg)
	if err != nil {
		logger.Fatal("Gmail init failed", zap.Error(err))
	}

	classifier, err := ai.NewClassifier(cfg, logger)
	if err != nil {
		logger.Fatal("Gemini init failed", zap.Error(err))
	}

	database, err := db.New(cfg)
	if err != nil {
		logger.Fatal("DB init failed", zap.Error(err))
	}

	if err := database.Migrate(context.Background()); err != nil {
		logger.Fatal("Migration failed", zap.Error(err))
	}

	// Gemini free tier: 60 RPM → safe at WORKER_COUNT=3
	limiter := rate.NewLimiter(
		rate.Limit(float64(cfg.WorkerCount)/60.0),
		cfg.WorkerCount,
	)

	return &Scheduler{
		cfg:         cfg,
		logger:      logger,
		cron:        cron.New(),
		classifier:  classifier,
		gmailClient: gmailClient,
		redisStore:  store.New(cfg),
		database:    database,
		slack:       notify.NewSlack(cfg),
		emailDigest: notify.NewEmailDigest(cfg),
		limiter:     limiter,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	// Poll for new emails
	s.cron.AddFunc(s.cfg.PollInterval, func() {
		s.runPipeline(ctx)
	})

	// Daily digest
	s.cron.AddFunc(s.cfg.DigestCron, func() {
		s.sendDigest(ctx)
	})

	// Weekly DB cleanup every Sunday midnight
	s.cron.AddFunc("0 0 * * 0", func() {
		n, err := s.database.Cleanup(ctx, 90)
		if err != nil {
			s.logger.Error("Cleanup error", zap.Error(err))
			return
		}
		s.logger.Info("Cleaned old rows", zap.Int64("deleted", n))
	})

	s.cron.Start()
	s.logger.Info("Scheduler started",
		zap.String("poll_interval", s.cfg.PollInterval),
		zap.String("digest_cron", s.cfg.DigestCron),
	)

	// Run once immediately on startup
	go s.runPipeline(ctx)
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
	s.classifier.Close()
	s.redisStore.Close()
	s.database.Close()
}

// Stats returns current run metrics (used by API dashboard)
func (s *Scheduler) Stats() (processed, failed int, lastRun time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.processed, s.failed, s.lastRun
}

// runPipeline fetches unread emails and classifies each one
func (s *Scheduler) runPipeline(ctx context.Context) {
	s.logger.Info("Pipeline run started")
	start := time.Now()

	emails, err := s.gmailClient.FetchUnread(ctx, s.cfg.MaxEmailsPerRun)
	if err != nil {
		s.logger.Error("FetchUnread failed", zap.Error(err))
		return
	}

	s.logger.Info("Fetched unread emails", zap.Int("count", len(emails)))

	sem := make(chan struct{}, s.cfg.WorkerCount)
	var wg sync.WaitGroup

	for _, email := range emails {
		email := email
		wg.Add(1)
		sem <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			s.processEmail(ctx, email)
		}()
	}

	wg.Wait()

	s.mu.Lock()
	s.lastRun = time.Now()
	s.mu.Unlock()

	s.logger.Info("Pipeline run complete",
		zap.Int("emails", len(emails)),
		zap.Duration("duration", time.Since(start)),
	)
}

// processEmail runs one email through the full triage pipeline
func (s *Scheduler) processEmail(ctx context.Context, email gmail.Email) {
	// 1. Dedup check
	processed, err := s.redisStore.IsProcessed(ctx, email.ID)
	if err != nil {
		s.logger.Warn("Redis check failed", zap.String("id", email.ID), zap.Error(err))
	}
	if processed {
		return
	}

	// 2. Rate-limit Gemini calls
	if err := s.limiter.Wait(ctx); err != nil {
		return
	}

	// 3. Fetch thread context if enabled
	var threadCtx []gmail.Email
	if s.cfg.EnableThreadCtx && email.ThreadID != "" {
		threadCtx, err = s.gmailClient.FetchThread(ctx, email.ThreadID)
		if err != nil {
			s.logger.Warn("FetchThread failed", zap.Error(err))
		}
	}

	// 4. Classify with Gemini
	clf, err := s.classifier.Classify(ctx, email, threadCtx)
	if err != nil {
		s.logger.Error("Classification failed",
			zap.String("id", email.ID),
			zap.Error(err),
		)
		s.mu.Lock()
		s.failed++
		s.mu.Unlock()
		return
	}

	// 5. Log to PostgreSQL
	if err := s.database.LogTriage(ctx, email, clf); err != nil {
		s.logger.Warn("DB log failed", zap.Error(err))
	}

	// 6. Mark as processed in Redis
	_ = s.redisStore.MarkProcessed(ctx, email.ID)

	// 7. Take actions (skip in dry run)
	if !s.cfg.DryRun {
		// Mark read in Gmail
		_ = s.gmailClient.MarkProcessed(ctx, email.ID)

		// Auto-archive LOW confidence emails
		if clf.Urgency == "LOW" && clf.Confidence >= s.cfg.AutoArchiveConf {
			if err := s.gmailClient.Archive(ctx, email.ID); err != nil {
				s.logger.Warn("Archive failed", zap.Error(err))
			}
		}

		// Auto-save draft reply
		if clf.ReplyDraft != "" && clf.Confidence >= s.cfg.AutoDraftConf {
			if err := s.gmailClient.CreateDraft(ctx, email, clf.ReplyDraft); err != nil {
				s.logger.Warn("CreateDraft failed", zap.Error(err))
			}
		}

		// Alert Slack for HIGH urgency
		if clf.Urgency == "HIGH" && clf.Confidence >= s.cfg.UrgencyHighConf {
			if err := s.slack.SendUrgentAlert(ctx, email, clf); err != nil {
				s.logger.Warn("Slack alert failed", zap.Error(err))
			}
		}
	}

	s.mu.Lock()
	s.processed++
	s.mu.Unlock()

	s.logger.Info("Email processed",
		zap.String("id", email.ID),
		zap.String("urgency", clf.Urgency),
		zap.String("topic", clf.Topic),
		zap.Bool("vip", clf.IsVIP),
		zap.Bool("dry_run", s.cfg.DryRun),
	)
}

// sendDigest collects stats and sends to Slack + email
func (s *Scheduler) sendDigest(ctx context.Context) {
	high, medium, low, total, err := s.database.Stats(ctx, 24)
	if err != nil {
		s.logger.Error("Stats failed", zap.Error(err))
		return
	}

	topTopics, err := s.database.TopTopics(ctx, 24, 5)
	if err != nil {
		s.logger.Warn("TopTopics failed", zap.Error(err))
	}

	// Slack digest
	if err := s.slack.SendDigest(ctx, total, high, medium, low, topTopics); err != nil {
		s.logger.Warn("Slack digest failed", zap.Error(err))
	}

	// Email digest — get recent HIGH emails for the table
	recentHigh, err := s.database.GetRecent(ctx, 10)
	if err != nil {
		s.logger.Warn("GetRecent failed", zap.Error(err))
	}

	if err := s.emailDigest.Send(ctx, total, high, medium, low, recentHigh); err != nil {
		s.logger.Warn("Email digest failed", zap.Error(err))
	}
}
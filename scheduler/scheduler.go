package scheduler

import (
    "context"
    "sync"

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
    limiter     *rate.Limiter
}

func New(cfg config.Config, logger *zap.Logger) *Scheduler {
    gmailClient, err := gmail.NewClient(cfg)
    if err != nil { logger.Fatal("Gmail init failed", zap.Error(err)) }
    classifier, err := ai.NewClassifier(cfg, logger)
    if err != nil { logger.Fatal("Gemini init failed", zap.Error(err)) }
    database, err := db.New(cfg)
    if err != nil { logger.Fatal("DB init failed", zap.Error(err)) }
    if err := database.Migrate(context.Background()); err != nil {
        logger.Fatal("Migration failed", zap.Error(err))
    }
    return &Scheduler{
        cfg:         cfg,
        logger:      logger,
        cron:        cron.New(),
        classifier:  classifier,
        gmailClient: gmailClient,
        redisStore:  store.New(cfg),
        database:    database,
        slack:       notify.NewSlack(cfg),
        limiter:     rate.NewLimiter(rate.Limit(float64(cfg.WorkerCount)/60.0), cfg.WorkerCount),
    }
}

func (s *Scheduler) Start(ctx context.Context) {
    s.cron.AddFunc(s.cfg.PollInterval, func() { s.runPipeline(ctx) })
    s.cron.AddFunc(s.cfg.DigestCron, func() { s.sendDigest(ctx) })
    s.cron.AddFunc("0 0 * * 0", func() {
        n, err := s.database.Cleanup(ctx, 90)
        if err != nil { s.logger.Error("Cleanup error", zap.Error(err)); return }
        s.logger.Info("Cleaned old rows", zap.Int64("deleted", n))
    })
    s.cron.Start()
    s.logger.Info("Scheduler started", zap.String("poll_interval", s.cfg.PollInterval))
    go s.runPipeline(ctx)
}

func (s *Scheduler) Stop() {
    s.cron.Stop()
    s.classifier.Close()
    s.database.Close()
}

func (s *Scheduler) runPipeline(ctx context.Context) {
    s.logger.Info("Pipeline run started")
    emails, err := s.gmailClient.FetchUnread(ctx, s.cfg.MaxEmailsPerRun)
    if err != nil { s.logger.Error("FetchUnread failed", zap.Error(err)); return }
    s.logger.Info("Fetched emails", zap.Int("count", len(emails)))
    sem := make(chan struct{}, s.cfg.WorkerCount)
    var wg sync.WaitGroup
    for _, email := range emails {
        email := email
        wg.Add(1); sem <- struct{}{}
        go func() {
            defer wg.Done(); defer func() { <-sem }()
            s.processEmail(ctx, email)
        }()
    }
    wg.Wait()
    s.logger.Info("Pipeline run complete")
}

func (s *Scheduler) processEmail(ctx context.Context, email gmail.Email) {
    processed, err := s.redisStore.IsProcessed(ctx, email.ID)
    if err != nil { s.logger.Warn("Redis check failed", zap.Error(err)) }
    if processed { return }
    if err := s.limiter.Wait(ctx); err != nil { return }
    clf, err := s.classifier.Classify(ctx, email)
    if err != nil { s.logger.Error("Classification failed", zap.String("id", email.ID), zap.Error(err)); return }
    _ = s.database.LogTriage(ctx, email, clf)
    _ = s.redisStore.MarkProcessed(ctx, email.ID)
    if !s.cfg.DryRun {
        _ = s.gmailClient.MarkProcessed(ctx, email.ID)
        if clf.Urgency == "LOW" && clf.Confidence >= s.cfg.AutoArchiveConf {
            _ = s.gmailClient.Archive(ctx, email.ID)
        }
        if clf.ReplyDraft != "" && clf.Confidence >= s.cfg.AutoDraftConf {
            _ = s.gmailClient.CreateDraft(ctx, email, clf.ReplyDraft)
        }
        if clf.Urgency == "HIGH" && clf.Confidence >= s.cfg.UrgencyHighConf {
            if err := s.slack.SendUrgentAlert(ctx, email, clf); err != nil {
                s.logger.Warn("Slack alert failed", zap.Error(err))
            }
        }
    }
    s.logger.Info("Processed", zap.String("id", email.ID), zap.String("urgency", clf.Urgency))
}

func (s *Scheduler) sendDigest(ctx context.Context) {
    high, medium, low, total, err := s.database.Stats(ctx, 24)
    if err != nil { s.logger.Error("Stats failed", zap.Error(err)); return }
    _ = s.slack.SendDigest(ctx, total, high, medium, low)
}

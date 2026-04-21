package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/session"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
)

type gatewayRuntimeOptions struct {
	ChatKey        string
	ResumeOverride string
	RecallEnabled  bool
}

type gatewayRuntime struct {
	SessionMap   *session.BoltMap
	MemoryStore  *memory.SqliteStore
	Kernel       *kernel.Kernel
	Extractor    *memory.Extractor
	Embedder     *memory.Embedder
	chatKey      string
	initialSID   string
	recallActive bool
}

func openGatewayRuntime(cfg config.Config, opt gatewayRuntimeOptions, log *slog.Logger) (*gatewayRuntime, error) {
	if log == nil {
		log = slog.Default()
	}

	smap, err := session.OpenBolt(config.SessionDBPath())
	if err != nil {
		return nil, fmt.Errorf("session map: %w", err)
	}

	ctx := context.Background()
	if opt.ChatKey != "" && opt.ResumeOverride != "" {
		if err := smap.Put(ctx, opt.ChatKey, opt.ResumeOverride); err != nil {
			_ = smap.Close()
			return nil, fmt.Errorf("apply resume override: %w", err)
		}
	}

	var initialSID string
	if opt.ChatKey != "" {
		if sid, err := smap.Get(ctx, opt.ChatKey); err == nil {
			initialSID = sid
			if sid != "" {
				log.Info("resuming persisted session", "key", opt.ChatKey, "session_id", sid)
			}
		}
	}

	mstore, err := memory.OpenSqlite(config.MemoryDBPath(), cfg.Telegram.MemoryQueueCap, log)
	if err != nil {
		_ = smap.Close()
		return nil, fmt.Errorf("memory store: %w", err)
	}

	mstore.StartMirror(memory.MirrorConfig{
		Enabled:  cfg.Telegram.MirrorEnabled,
		Path:     cfg.Telegram.MirrorPath,
		Interval: cfg.Telegram.MirrorInterval,
		Logger:   log,
	})

	hc := hermes.NewHTTPClient(cfg.Hermes.Endpoint, cfg.Hermes.APIKey)

	reg := buildDefaultRegistry()
	registerDelegation(cfg, reg, hc)

	tm := telemetry.New()

	recallActive := opt.RecallEnabled && opt.ChatKey != ""

	var (
		recallProv kernel.RecallProvider
		semCache   *memory.SemanticCache
		ec         *memory.EmbedClient
	)

	if recallActive && cfg.Telegram.SemanticEnabled && cfg.Telegram.SemanticModel != "" {
		endpoint := cfg.Telegram.SemanticEndpoint
		if endpoint == "" {
			endpoint = cfg.Hermes.Endpoint
		}
		ec = memory.NewEmbedClient(endpoint, cfg.Hermes.APIKey)
		semCache = memory.NewSemanticCache()
	}

	if recallActive {
		memProv := memory.NewRecall(mstore, memory.RecallConfig{
			WeightThreshold:       cfg.Telegram.RecallWeightThreshold,
			MaxFacts:              cfg.Telegram.RecallMaxFacts,
			Depth:                 cfg.Telegram.RecallDepth,
			DecayHorizonDays:      cfg.Telegram.RecallDecayHorizonDays,
			SemanticModel:         cfg.Telegram.SemanticModel,
			SemanticTopK:          cfg.Telegram.SemanticTopK,
			SemanticMinSimilarity: cfg.Telegram.SemanticMinSimilarity,
			QueryEmbedTimeout:     cfg.Telegram.QueryEmbedTimeout,
		}, log)
		if ec != nil {
			memProv = memProv.WithEmbedClient(ec, semCache)
		}
		recallProv = &recallAdapter{p: memProv}
	}

	k := kernel.New(kernel.Config{
		Model:             cfg.Hermes.Model,
		Endpoint:          cfg.Hermes.Endpoint,
		Admission:         kernel.Admission{MaxBytes: cfg.Input.MaxBytes, MaxLines: cfg.Input.MaxLines},
		Tools:             reg,
		MaxToolIterations: 10,
		MaxToolDuration:   30 * time.Second,
		InitialSessionID:  initialSID,
		Recall:            recallProv,
		ChatKey:           opt.ChatKey,
	}, hc, mstore, tm, log)

	ext := memory.NewExtractor(mstore, hc, memory.ExtractorConfig{
		Model:        cfg.Hermes.Model,
		BatchSize:    cfg.Telegram.ExtractorBatchSize,
		PollInterval: cfg.Telegram.ExtractorPollInterval,
	}, log)

	var embedder *memory.Embedder
	if ec != nil {
		embedder = memory.NewEmbedder(mstore, ec, memory.EmbedderConfig{
			Model:        cfg.Telegram.SemanticModel,
			PollInterval: cfg.Telegram.EmbedderPollInterval,
			BatchSize:    cfg.Telegram.EmbedderBatchSize,
			CallTimeout:  cfg.Telegram.EmbedderCallTimeout,
		}, log, semCache)
	}

	return &gatewayRuntime{
		SessionMap:   smap,
		MemoryStore:  mstore,
		Kernel:       k,
		Extractor:    ext,
		Embedder:     embedder,
		chatKey:      opt.ChatKey,
		initialSID:   initialSID,
		recallActive: recallActive,
	}, nil
}

func (rt *gatewayRuntime) Start(ctx context.Context) {
	if rt == nil {
		return
	}
	if rt.Kernel != nil {
		go rt.Kernel.Run(ctx)
	}
	if rt.Extractor != nil {
		go rt.Extractor.Run(ctx)
	}
	if rt.Embedder != nil {
		go rt.Embedder.Run(ctx)
	}
}

func (rt *gatewayRuntime) Close(ctx context.Context) {
	if rt == nil {
		return
	}
	if rt.Embedder != nil {
		if err := rt.Embedder.Close(ctx); err != nil {
			slog.Warn("embedder close", "err", err)
		}
	}
	if rt.Extractor != nil {
		if err := rt.Extractor.Close(ctx); err != nil {
			slog.Warn("extractor close", "err", err)
		}
	}
	if rt.MemoryStore != nil {
		if err := rt.MemoryStore.Close(ctx); err != nil {
			slog.Warn("memory store close", "err", err)
		}
	}
	if rt.SessionMap != nil {
		_ = rt.SessionMap.Close()
	}
}

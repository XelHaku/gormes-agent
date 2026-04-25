package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/spf13/cobra"
	bolt "go.etcd.io/bbolt"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/doctor"
	"github.com/TrebuchetDynamics/gormes-agent/internal/goncho"
	"github.com/TrebuchetDynamics/gormes-agent/internal/gonchotools"
	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/internal/tools"
)

var gonchoCmd = &cobra.Command{
	Use:   "goncho",
	Short: "Inspect local Goncho memory diagnostics",
}

var gonchoDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose local Goncho memory topology, queues, and degraded modes",
	Args:  cobra.NoArgs,
	RunE:  runGonchoDoctor,
}

func init() {
	gonchoDoctorCmd.Flags().Bool("json", false, "emit machine-readable JSON")
	gonchoDoctorCmd.Flags().String("peer", "operator:diagnostic", "peer id for the context dry-run")
	gonchoDoctorCmd.Flags().String("session", "", "optional session key for the context dry-run")
	gonchoDoctorCmd.Flags().Bool("require-provider", false, "treat provider/auth readiness as required for this diagnostic")
	gonchoCmd.AddCommand(gonchoDoctorCmd)
}

type gonchoDoctorReport struct {
	Service                string                 `json:"service"`
	Status                 string                 `json:"status"`
	ExitCode               int                    `json:"exit_code"`
	Config                 gonchoDoctorConfig     `json:"config"`
	Schema                 memory.SchemaStatus    `json:"schema"`
	SessionCatalog         sessionCatalogStatus   `json:"session_catalog"`
	ToolRegistration       toolRegistrationStatus `json:"tool_registration"`
	ContextDryRun          contextDryRunStatus    `json:"context_dry_run"`
	QueueStatus            doctorQueueStatus      `json:"queue_status"`
	ConclusionAvailability conclusionAvailability `json:"conclusion_availability"`
	SummaryAvailability    summaryAvailability    `json:"summary_availability"`
	ProviderReadiness      providerReadiness      `json:"provider_readiness"`
	DegradedModes          []degradedMode         `json:"degraded_modes"`
}

type gonchoDoctorConfig struct {
	ConfigPath     string `json:"config_path"`
	ConfigExists   bool   `json:"config_exists"`
	MemoryDBPath   string `json:"memory_db_path"`
	SessionDBPath  string `json:"session_db_path"`
	Workspace      string `json:"workspace"`
	ObserverPeer   string `json:"observer_peer"`
	RecentMessages int    `json:"recent_messages"`
	HermesModel    string `json:"hermes_model"`
}

type sessionCatalogStatus struct {
	Status        string `json:"status"`
	Path          string `json:"path"`
	Exists        bool   `json:"exists"`
	SessionCount  int    `json:"session_count"`
	MetadataCount int    `json:"metadata_count"`
	Message       string `json:"message"`
}

type toolRegistrationStatus struct {
	Status     string   `json:"status"`
	Summary    string   `json:"summary"`
	Registered []string `json:"registered"`
	Invalid    []string `json:"invalid,omitempty"`
}

type contextDryRunStatus struct {
	Status         string                              `json:"status"`
	Peer           string                              `json:"peer"`
	SessionKey     string                              `json:"session_key"`
	Representation string                              `json:"representation"`
	Unavailable    []goncho.ContextUnavailableEvidence `json:"unavailable"`
}

type extractorQueueSnapshot struct {
	WorkerHealth     string `json:"worker_health"`
	QueueDepth       int    `json:"queue_depth"`
	DeadLetterCount  int    `json:"dead_letter_count"`
	SkippedSyncCount int    `json:"skipped_sync_count"`
}

type doctorQueueStatus struct {
	Status            string                                `json:"status"`
	ObservabilityOnly bool                                  `json:"observability_only"`
	Extractor         extractorQueueSnapshot                `json:"extractor"`
	WorkUnits         map[string]goncho.QueueWorkUnitStatus `json:"work_units"`
	Message           string                                `json:"message"`
}

type conclusionAvailability struct {
	Status string           `json:"status"`
	Total  int              `json:"total"`
	Pairs  []conclusionPair `json:"pairs"`
}

type conclusionPair struct {
	ObserverPeerID string `json:"observer_peer_id"`
	PeerID         string `json:"peer_id"`
	Count          int    `json:"count"`
}

type summaryAvailability struct {
	Status       string `json:"status"`
	TablePresent bool   `json:"table_present"`
	Total        int    `json:"total"`
	Message      string `json:"message"`
}

type providerReadiness struct {
	Status   string `json:"status"`
	Required bool   `json:"required"`
	Checked  bool   `json:"checked"`
	Message  string `json:"message"`
}

type degradedMode struct {
	Capability string `json:"capability"`
	Severity   string `json:"severity"`
	Reason     string `json:"reason"`
}

func runGonchoDoctor(cmd *cobra.Command, _ []string) error {
	emitJSON, _ := cmd.Flags().GetBool("json")
	peer, _ := cmd.Flags().GetString("peer")
	sessionKey, _ := cmd.Flags().GetString("session")
	requireProvider, _ := cmd.Flags().GetBool("require-provider")

	peer = strings.TrimSpace(peer)
	sessionKey = strings.TrimSpace(sessionKey)
	if peer == "" {
		return newExitCodeError(1, errors.New("goncho doctor: --peer is required"))
	}

	cfg, err := config.Load(nil)
	if err != nil {
		return newExitCodeError(1, err)
	}

	memoryPath := config.MemoryDBPath()
	if _, err := os.Stat(memoryPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newExitCodeError(1, fmt.Errorf("memory database not found at %s", memoryPath))
		}
		return newExitCodeError(1, err)
	}

	db, err := sql.Open("sqlite3", memoryPath)
	if err != nil {
		return newExitCodeError(2, fmt.Errorf("open memory db: %w", err))
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	report, code, err := buildGonchoDoctorReport(ctx, cfg, db, peer, sessionKey, requireProvider)
	if err != nil {
		return newExitCodeError(code, err)
	}

	if emitJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprint(cmd.OutOrStdout(), formatGonchoDoctorReport(report)); err != nil {
			return err
		}
	}

	if report.ExitCode != 0 {
		return newExitCodeError(report.ExitCode, fmt.Errorf("goncho doctor: %s", report.Status))
	}
	return nil
}

func buildGonchoDoctorReport(ctx context.Context, cfg config.Config, db *sql.DB, peer, sessionKey string, requireProvider bool) (gonchoDoctorReport, int, error) {
	schema, err := memory.ReadSchemaStatus(ctx, db)
	if err != nil {
		return gonchoDoctorReport{}, 2, err
	}
	if !schema.Current || !requiredSchemaTablesPresent(schema.Tables) {
		report := gonchoDoctorReport{
			Service:  "goncho",
			Status:   "runtime_storage_error",
			ExitCode: 2,
			Config:   currentGonchoDoctorConfig(cfg),
			Schema:   schema,
		}
		return report, 2, nil
	}

	extractor, err := memory.ReadExtractorStatus(ctx, db, 5)
	if err != nil {
		return gonchoDoctorReport{}, 2, err
	}
	queue, err := goncho.ReadQueueStatus(ctx, db)
	if err != nil {
		return gonchoDoctorReport{}, 2, err
	}

	svc := goncho.NewService(db, goncho.Config{}, nil)
	toolStatus := readToolRegistration(svc)
	contextStatus, err := readContextDryRun(ctx, svc, peer, sessionKey)
	if err != nil {
		return gonchoDoctorReport{}, 2, err
	}
	conclusions, err := readConclusionAvailability(ctx, db)
	if err != nil {
		return gonchoDoctorReport{}, 2, err
	}
	summaries, err := readSummaryAvailability(ctx, db)
	if err != nil {
		return gonchoDoctorReport{}, 2, err
	}
	sessionCatalog, err := readSessionCatalogStatus(config.SessionDBPath())
	if err != nil {
		return gonchoDoctorReport{}, 2, err
	}
	provider := readProviderReadiness(cfg, requireProvider)

	degraded := collectGonchoDegradedModes(queue, summaries, provider)
	exitCode := 0
	status := "healthy"
	if len(degraded) > 0 {
		status = "degraded"
	}
	if toolStatus.Status == "fail" {
		exitCode = 2
		status = "runtime_storage_error"
	}
	if provider.Status == "fail" {
		exitCode = 3
		status = "auth_provider_error"
	}

	report := gonchoDoctorReport{
		Service:          "goncho",
		Status:           status,
		ExitCode:         exitCode,
		Config:           currentGonchoDoctorConfig(cfg),
		Schema:           schema,
		SessionCatalog:   sessionCatalog,
		ToolRegistration: toolStatus,
		ContextDryRun:    contextStatus,
		QueueStatus: doctorQueueStatus{
			Status:            queue.Status,
			ObservabilityOnly: queue.ObservabilityOnly,
			Extractor: extractorQueueSnapshot{
				WorkerHealth:     extractor.WorkerHealth,
				QueueDepth:       extractor.QueueDepth,
				DeadLetterCount:  extractor.DeadLetterCount,
				SkippedSyncCount: extractor.SkippedSyncCount,
			},
			WorkUnits: queue.WorkUnits,
			Message:   queue.Message,
		},
		ConclusionAvailability: conclusions,
		SummaryAvailability:    summaries,
		ProviderReadiness:      provider,
		DegradedModes:          degraded,
	}
	return report, report.ExitCode, nil
}

func currentGonchoDoctorConfig(cfg config.Config) gonchoDoctorConfig {
	configPath := config.ConfigPath()
	_, err := os.Stat(configPath)
	return gonchoDoctorConfig{
		ConfigPath:     configPath,
		ConfigExists:   err == nil,
		MemoryDBPath:   config.MemoryDBPath(),
		SessionDBPath:  config.SessionDBPath(),
		Workspace:      goncho.DefaultWorkspaceID,
		ObserverPeer:   goncho.DefaultObserverPeerID,
		RecentMessages: 4,
		HermesModel:    cfg.Hermes.Model,
	}
}

func requiredSchemaTablesPresent(tables map[string]bool) bool {
	for _, table := range []string{"turns", "turns_fts", "goncho_peer_cards", "goncho_conclusions", "goncho_conclusions_fts"} {
		if !tables[table] {
			return false
		}
	}
	return true
}

func readToolRegistration(svc *goncho.Service) toolRegistrationStatus {
	reg := tools.NewRegistry()
	gonchotools.RegisterHonchoTools(reg, svc)
	result := doctor.CheckTools(reg)

	out := toolRegistrationStatus{
		Status:  "ok",
		Summary: result.Summary,
	}
	if result.Status == doctor.StatusFail {
		out.Status = "fail"
	}
	for _, item := range result.Items {
		out.Registered = append(out.Registered, item.Name)
		if item.Status == doctor.StatusFail {
			out.Invalid = append(out.Invalid, item.Name)
		}
	}
	return out
}

func readContextDryRun(ctx context.Context, svc *goncho.Service, peer, sessionKey string) (contextDryRunStatus, error) {
	result, err := svc.Context(ctx, goncho.ContextParams{
		Peer:       peer,
		Query:      "doctor dry-run",
		MaxTokens:  400,
		SessionKey: sessionKey,
	})
	if err != nil {
		return contextDryRunStatus{}, err
	}
	return contextDryRunStatus{
		Status:         "ok",
		Peer:           result.Peer,
		SessionKey:     result.SessionKey,
		Representation: result.Representation,
		Unavailable:    result.Unavailable,
	}, nil
}

func readConclusionAvailability(ctx context.Context, db *sql.DB) (conclusionAvailability, error) {
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM goncho_conclusions`).Scan(&total); err != nil {
		return conclusionAvailability{}, fmt.Errorf("goncho doctor: conclusion count: %w", err)
	}
	status := "ok"
	if total == 0 {
		status = "zero_state"
	}
	out := conclusionAvailability{Status: status, Total: total}

	rows, err := db.QueryContext(ctx, `
		SELECT observer_peer_id, peer_id, COUNT(*)
		FROM goncho_conclusions
		GROUP BY observer_peer_id, peer_id
		ORDER BY COUNT(*) DESC, observer_peer_id, peer_id
		LIMIT 10
	`)
	if err != nil {
		return conclusionAvailability{}, fmt.Errorf("goncho doctor: conclusion pairs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var pair conclusionPair
		if err := rows.Scan(&pair.ObserverPeerID, &pair.PeerID, &pair.Count); err != nil {
			return conclusionAvailability{}, fmt.Errorf("goncho doctor: scan conclusion pair: %w", err)
		}
		out.Pairs = append(out.Pairs, pair)
	}
	if err := rows.Err(); err != nil {
		return conclusionAvailability{}, fmt.Errorf("goncho doctor: conclusion pair rows: %w", err)
	}
	return out, nil
}

func readSummaryAvailability(ctx context.Context, db *sql.DB) (summaryAvailability, error) {
	present, err := sqliteTablePresent(ctx, db, "goncho_session_summaries")
	if err != nil {
		return summaryAvailability{}, err
	}
	if !present {
		return summaryAvailability{
			Status:       "degraded",
			TablePresent: false,
			Message:      "goncho_session_summaries table unavailable; summary capability is degraded",
		}, nil
	}

	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM goncho_session_summaries`).Scan(&total); err != nil {
		return summaryAvailability{}, fmt.Errorf("goncho doctor: summary count: %w", err)
	}
	status := "ok"
	if total == 0 {
		status = "zero_state"
	}
	return summaryAvailability{Status: status, TablePresent: true, Total: total}, nil
}

func readSessionCatalogStatus(path string) (sessionCatalogStatus, error) {
	out := sessionCatalogStatus{
		Status:  "zero_state",
		Path:    path,
		Message: "no session catalog data",
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return sessionCatalogStatus{}, err
	}
	out.Exists = true

	db, err := bolt.Open(path, 0o600, &bolt.Options{ReadOnly: true, Timeout: 100 * time.Millisecond})
	if err != nil {
		return sessionCatalogStatus{}, fmt.Errorf("session catalog: open %s: %w", path, err)
	}
	defer db.Close()

	if err := db.View(func(tx *bolt.Tx) error {
		out.SessionCount = countBoltBucket(tx.Bucket([]byte("sessions_v1")))
		out.MetadataCount = countBoltBucket(tx.Bucket([]byte("session_meta_v1")))
		return nil
	}); err != nil {
		return sessionCatalogStatus{}, err
	}
	if out.SessionCount > 0 || out.MetadataCount > 0 {
		out.Status = "ok"
		out.Message = "session catalog readable"
	}
	return out, nil
}

func countBoltBucket(bucket *bolt.Bucket) int {
	if bucket == nil {
		return 0
	}
	count := 0
	_ = bucket.ForEach(func(_, _ []byte) error {
		count++
		return nil
	})
	return count
}

func readProviderReadiness(cfg config.Config, required bool) providerReadiness {
	if required {
		if strings.TrimSpace(cfg.Hermes.APIKey) == "" {
			return providerReadiness{
				Status:   "fail",
				Required: true,
				Checked:  true,
				Message:  "provider auth required but GORMES_API_KEY is not configured",
			}
		}
		return providerReadiness{
			Status:   "ok",
			Required: true,
			Checked:  true,
			Message:  "provider auth is configured; network reachability is not checked by Goncho doctor",
		}
	}
	return providerReadiness{
		Status:   "degraded",
		Required: false,
		Checked:  false,
		Message:  "missing optional model/provider features are degraded, not startup failures; no provider network check was run",
	}
}

func collectGonchoDegradedModes(queue goncho.QueueStatus, summaries summaryAvailability, provider providerReadiness) []degradedMode {
	var out []degradedMode
	if queue.Degraded {
		out = append(out, degradedMode{
			Capability: "goncho_task_queue",
			Severity:   "degraded",
			Reason:     queue.Message,
		})
	}
	if summaries.Status == "degraded" {
		out = append(out, degradedMode{
			Capability: "session_summaries",
			Severity:   "degraded",
			Reason:     summaries.Message,
		})
	}
	if provider.Status == "degraded" {
		out = append(out, degradedMode{
			Capability: "model_provider",
			Severity:   "degraded",
			Reason:     provider.Message,
		})
	}
	return out
}

func sqliteTablePresent(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var found string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sqlite table %s: %w", name, err)
	}
	return found == name, nil
}

func formatGonchoDoctorReport(report gonchoDoctorReport) string {
	var b strings.Builder
	b.WriteString("Goncho doctor\n")
	fmt.Fprintf(&b, "status: %s\n", report.Status)
	fmt.Fprintf(&b, "exit_code: %d\n\n", report.ExitCode)

	b.WriteString("Config\n")
	fmt.Fprintf(&b, "config_path: %s\n", report.Config.ConfigPath)
	fmt.Fprintf(&b, "config_exists: %t\n", report.Config.ConfigExists)
	fmt.Fprintf(&b, "memory_db_path: %s\n", report.Config.MemoryDBPath)
	fmt.Fprintf(&b, "session_db_path: %s\n", report.Config.SessionDBPath)
	fmt.Fprintf(&b, "workspace: %s\n", report.Config.Workspace)
	fmt.Fprintf(&b, "observer_peer: %s\n", report.Config.ObserverPeer)
	fmt.Fprintf(&b, "recent_messages: %d\n\n", report.Config.RecentMessages)

	b.WriteString("Schema\n")
	fmt.Fprintf(&b, "schema_version: %s\n", report.Schema.Version)
	fmt.Fprintf(&b, "current_schema_version: %s\n", report.Schema.CurrentVersion)
	for _, table := range []string{"turns", "turns_fts", "goncho_peer_cards", "goncho_conclusions", "goncho_conclusions_fts"} {
		fmt.Fprintf(&b, "%s: %s\n", table, presentWord(report.Schema.Tables[table]))
	}
	b.WriteString("\n")

	b.WriteString("Session catalog\n")
	fmt.Fprintf(&b, "path: %s\n", report.SessionCatalog.Path)
	fmt.Fprintf(&b, "status: %s\n", report.SessionCatalog.Status)
	fmt.Fprintf(&b, "sessions: %d\n", report.SessionCatalog.SessionCount)
	fmt.Fprintf(&b, "metadata_rows: %d\n", report.SessionCatalog.MetadataCount)
	fmt.Fprintf(&b, "%s\n\n", report.SessionCatalog.Message)

	b.WriteString("Tool registration\n")
	fmt.Fprintf(&b, "status: %s\n", report.ToolRegistration.Status)
	fmt.Fprintf(&b, "summary: %s\n", report.ToolRegistration.Summary)
	fmt.Fprintf(&b, "tools: %s\n\n", strings.Join(report.ToolRegistration.Registered, ", "))

	b.WriteString("Context dry-run\n")
	fmt.Fprintf(&b, "peer: %s\n", report.ContextDryRun.Peer)
	fmt.Fprintf(&b, "session_key: %s\n", valueOrNone(report.ContextDryRun.SessionKey))
	fmt.Fprintf(&b, "representation: %s\n", report.ContextDryRun.Representation)
	if len(report.ContextDryRun.Unavailable) == 0 {
		b.WriteString("unavailable: none\n\n")
	} else {
		b.WriteString("unavailable:\n")
		for _, item := range report.ContextDryRun.Unavailable {
			fmt.Fprintf(&b, "- %s: %s\n", item.Capability, item.Reason)
		}
		b.WriteString("\n")
	}

	b.WriteString("Queue status (observability/debugging only; not synchronization; do not wait for empty queue)\n")
	fmt.Fprintf(&b, "extractor_worker_health: %s\n", report.QueueStatus.Extractor.WorkerHealth)
	fmt.Fprintf(&b, "extractor_queue_depth: %d\n", report.QueueStatus.Extractor.QueueDepth)
	fmt.Fprintf(&b, "extractor_dead_letters: %d\n", report.QueueStatus.Extractor.DeadLetterCount)
	for _, taskType := range goncho.QueueTaskTypes {
		counts := report.QueueStatus.WorkUnits[taskType]
		fmt.Fprintf(&b, "%s: total=%d pending=%d in_progress=%d completed=%d\n",
			taskType,
			counts.TotalWorkUnits,
			counts.PendingWorkUnits,
			counts.InProgressWorkUnits,
			counts.CompletedWorkUnits,
		)
	}
	fmt.Fprintf(&b, "goncho_queue: %s\n\n", report.QueueStatus.Message)

	b.WriteString("Conclusion availability\n")
	fmt.Fprintf(&b, "status: %s\n", report.ConclusionAvailability.Status)
	fmt.Fprintf(&b, "conclusion_count: %d\n\n", report.ConclusionAvailability.Total)

	b.WriteString("Summary availability\n")
	fmt.Fprintf(&b, "status: %s\n", report.SummaryAvailability.Status)
	fmt.Fprintf(&b, "summary_table: %s\n", availableWord(report.SummaryAvailability.TablePresent))
	fmt.Fprintf(&b, "summary_count: %d\n\n", report.SummaryAvailability.Total)

	b.WriteString("Provider readiness\n")
	fmt.Fprintf(&b, "status: %s\n", report.ProviderReadiness.Status)
	fmt.Fprintf(&b, "required: %t\n", report.ProviderReadiness.Required)
	fmt.Fprintf(&b, "checked: %t\n", report.ProviderReadiness.Checked)
	fmt.Fprintf(&b, "optional_provider_checks: %s\n", report.ProviderReadiness.Status)
	fmt.Fprintf(&b, "message: %s\n\n", report.ProviderReadiness.Message)

	b.WriteString("Degraded modes\n")
	if len(report.DegradedModes) == 0 {
		b.WriteString("none\n")
		return b.String()
	}
	for _, item := range report.DegradedModes {
		fmt.Fprintf(&b, "- %s: %s (%s)\n", item.Capability, item.Severity, item.Reason)
	}
	return b.String()
}

func presentWord(ok bool) string {
	if ok {
		return "present"
	}
	return "missing"
}

func availableWord(ok bool) string {
	if ok {
		return "available"
	}
	return "unavailable"
}

func valueOrNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}

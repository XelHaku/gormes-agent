package apiserver

const (
	detailedHealthStatusReady    = "ready"
	detailedHealthStatusDegraded = "degraded"

	detailedHealthProviderUnconfigured  = "provider_unconfigured"
	detailedHealthResponseStoreDisabled = "response_store_disabled"
	detailedHealthRunEventsUnavailable  = "run_events_unavailable"
	detailedHealthGatewayUnavailable    = "gateway_unavailable"
	detailedHealthCronUnavailable       = "cron_unavailable"
)

// DetailedHealthSnapshotInput is a value-only health read model. Callers fill
// it from already-available status reads; this package does not bind routes,
// start schedulers, contact providers, or mutate response/run/cron stores.
type DetailedHealthSnapshotInput struct {
	Provider      DetailedHealthProviderInput
	ResponseStore DetailedHealthResponseStoreInput
	RunEvents     DetailedHealthRunEventsInput
	Gateway       DetailedHealthGatewayInput
	Cron          DetailedHealthCronInput
}

type DetailedHealthProviderInput struct {
	Name       string
	Model      string
	Configured bool

	APIKey            string
	RawRequestPayload string
}

type DetailedHealthResponseStoreInput struct {
	Enabled      bool
	Stored       int
	MaxStored    int
	LRUEvictions int
}

type DetailedHealthRunEventsInput struct {
	Available     bool
	Active        int
	OrphanedSwept int
	TTLSeconds    int
}

type DetailedHealthGatewayInput struct {
	Available    bool
	State        string
	ActiveAgents int
	Platforms    map[string]string
	ProxyState   string

	Token string
}

type DetailedHealthCronInput struct {
	Available     bool
	Enabled       bool
	Jobs          int
	Paused        int
	LastRunStatus string

	ScriptBodies []string
}

type DetailedHealthSnapshotModel struct {
	Provider      DetailedHealthProviderSection      `json:"provider"`
	ResponseStore DetailedHealthResponseStoreSection `json:"response_store"`
	RunEvents     DetailedHealthRunEventsSection     `json:"run_events"`
	Gateway       DetailedHealthGatewaySection       `json:"gateway"`
	Cron          DetailedHealthCronSection          `json:"cron"`
}

type DetailedHealthEvidence struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type DetailedHealthProviderSection struct {
	Status   string                   `json:"status"`
	Name     string                   `json:"name"`
	Model    string                   `json:"model"`
	Evidence []DetailedHealthEvidence `json:"evidence"`
}

type DetailedHealthResponseStoreSection struct {
	Status       string                   `json:"status"`
	StoreEnabled bool                     `json:"store_enabled"`
	Stored       int                      `json:"stored"`
	MaxStored    int                      `json:"max_stored"`
	LRUEvictions int                      `json:"lru_evictions"`
	Evidence     []DetailedHealthEvidence `json:"evidence"`
}

type DetailedHealthRunEventsSection struct {
	Status        string                   `json:"status"`
	Available     bool                     `json:"available"`
	Active        int                      `json:"active"`
	OrphanedSwept int                      `json:"orphaned_swept"`
	TTLSeconds    int                      `json:"ttl_seconds"`
	Evidence      []DetailedHealthEvidence `json:"evidence"`
}

type DetailedHealthGatewaySection struct {
	Status       string                   `json:"status"`
	Available    bool                     `json:"available"`
	GatewayState string                   `json:"gateway_state"`
	ActiveAgents int                      `json:"active_agents"`
	Platforms    map[string]string        `json:"platforms"`
	ProxyState   string                   `json:"proxy_state"`
	Evidence     []DetailedHealthEvidence `json:"evidence"`
}

type DetailedHealthCronSection struct {
	Status        string                   `json:"status"`
	Available     bool                     `json:"available"`
	Enabled       bool                     `json:"enabled"`
	Jobs          int                      `json:"jobs"`
	Paused        int                      `json:"paused"`
	LastRunStatus string                   `json:"last_run_status"`
	Evidence      []DetailedHealthEvidence `json:"evidence"`
}

func DetailedHealthSnapshot(input DetailedHealthSnapshotInput) DetailedHealthSnapshotModel {
	return DetailedHealthSnapshotModel{
		Provider:      detailedHealthProvider(input.Provider),
		ResponseStore: detailedHealthResponseStore(input.ResponseStore),
		RunEvents:     detailedHealthRunEvents(input.RunEvents),
		Gateway:       detailedHealthGateway(input.Gateway),
		Cron:          detailedHealthCron(input.Cron),
	}
}

func detailedHealthProvider(input DetailedHealthProviderInput) DetailedHealthProviderSection {
	section := DetailedHealthProviderSection{
		Status:   detailedHealthStatusReady,
		Name:     input.Name,
		Model:    input.Model,
		Evidence: []DetailedHealthEvidence{},
	}
	if !input.Configured {
		section.Status = detailedHealthStatusDegraded
		section.Evidence = append(section.Evidence, DetailedHealthEvidence{
			Code:    detailedHealthProviderUnconfigured,
			Message: "provider configuration is not available",
		})
	}
	return section
}

func detailedHealthResponseStore(input DetailedHealthResponseStoreInput) DetailedHealthResponseStoreSection {
	section := DetailedHealthResponseStoreSection{
		Status:       detailedHealthStatusReady,
		StoreEnabled: input.Enabled,
		Stored:       input.Stored,
		MaxStored:    input.MaxStored,
		LRUEvictions: input.LRUEvictions,
		Evidence:     []DetailedHealthEvidence{},
	}
	if !input.Enabled {
		section.Status = detailedHealthStatusDegraded
		section.Evidence = append(section.Evidence, DetailedHealthEvidence{
			Code:    detailedHealthResponseStoreDisabled,
			Message: "response store is disabled",
		})
	}
	return section
}

func detailedHealthRunEvents(input DetailedHealthRunEventsInput) DetailedHealthRunEventsSection {
	section := DetailedHealthRunEventsSection{
		Status:        detailedHealthStatusReady,
		Available:     input.Available,
		Active:        input.Active,
		OrphanedSwept: input.OrphanedSwept,
		TTLSeconds:    input.TTLSeconds,
		Evidence:      []DetailedHealthEvidence{},
	}
	if !input.Available {
		section.Status = detailedHealthStatusDegraded
		section.Evidence = append(section.Evidence, DetailedHealthEvidence{
			Code:    detailedHealthRunEventsUnavailable,
			Message: "run event stream is unavailable",
		})
	}
	return section
}

func detailedHealthGateway(input DetailedHealthGatewayInput) DetailedHealthGatewaySection {
	section := DetailedHealthGatewaySection{
		Status:       detailedHealthStatusReady,
		Available:    input.Available,
		GatewayState: input.State,
		ActiveAgents: input.ActiveAgents,
		Platforms:    cloneDetailedHealthPlatforms(input.Platforms),
		ProxyState:   input.ProxyState,
		Evidence:     []DetailedHealthEvidence{},
	}
	if !input.Available {
		section.Status = detailedHealthStatusDegraded
		section.Evidence = append(section.Evidence, DetailedHealthEvidence{
			Code:    detailedHealthGatewayUnavailable,
			Message: "gateway runtime status is unavailable",
		})
	}
	return section
}

func detailedHealthCron(input DetailedHealthCronInput) DetailedHealthCronSection {
	section := DetailedHealthCronSection{
		Status:        detailedHealthStatusReady,
		Available:     input.Available,
		Enabled:       input.Enabled,
		Jobs:          input.Jobs,
		Paused:        input.Paused,
		LastRunStatus: input.LastRunStatus,
		Evidence:      []DetailedHealthEvidence{},
	}
	if !input.Available {
		section.Status = detailedHealthStatusDegraded
		section.Evidence = append(section.Evidence, DetailedHealthEvidence{
			Code:    detailedHealthCronUnavailable,
			Message: "cron store status is unavailable",
		})
	}
	return section
}

func cloneDetailedHealthPlatforms(platforms map[string]string) map[string]string {
	out := make(map[string]string, len(platforms))
	for name, state := range platforms {
		out[name] = state
	}
	return out
}

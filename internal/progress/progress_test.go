package progress

import (
	"path/filepath"
	"strings"
	"testing"
)

func itemStatusByName(items []Item) map[string]Status {
	out := make(map[string]Status, len(items))
	for _, it := range items {
		out[it.Name] = it.Status
	}
	return out
}

func itemsByName(items []Item) map[string]Item {
	out := make(map[string]Item, len(items))
	for _, it := range items {
		out[it.Name] = it
	}
	return out
}

func TestLoad_MinimalFixture(t *testing.T) {
	p, err := Load(filepath.Join("testdata", "minimal.json"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if p.Meta.Version != "2.0" {
		t.Errorf("Meta.Version = %q, want %q", p.Meta.Version, "2.0")
	}
	if p.Meta.LastUpdated != "2026-04-20" {
		t.Errorf("Meta.LastUpdated = %q, want %q", p.Meta.LastUpdated, "2026-04-20")
	}
	ph, ok := p.Phases["1"]
	if !ok {
		t.Fatalf("Phases[\"1\"] missing")
	}
	sp, ok := ph.Subphases["1.A"]
	if !ok {
		t.Fatalf("Subphases[\"1.A\"] missing")
	}
	if len(sp.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(sp.Items))
	}
	if sp.Items[0].Name != "item one" || sp.Items[0].Status != StatusComplete {
		t.Errorf("items[0] = %+v, want name=item one status=complete", sp.Items[0])
	}
}

func TestLoad_RealFile(t *testing.T) {
	p, err := Load("../../docs/content/building-gormes/architecture_plan/progress.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := Validate(p); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	// Phase 1 has all dashboard and automation-reliability rows shipped.
	if got := p.Phases["1"].DerivedStatus(); got != StatusComplete {
		t.Errorf("Phase 1 = %q, want complete", got)
	}
	// Phase 2 has 2.A, 2.B.1, 2.C complete and more planned -> in_progress.
	if got := p.Phases["2"].DerivedStatus(); got != StatusInProgress {
		t.Errorf("Phase 2 = %q, want in_progress", got)
	}
	// Phase 3 has most memory subphases shipped, 3.E.* planned -> in_progress.
	if got := p.Phases["3"].DerivedStatus(); got != StatusInProgress {
		t.Errorf("Phase 3 = %q, want in_progress", got)
	}
	// Phase 4 has the Anthropic adapter landed while the rest stays planned.
	if got := p.Phases["4"].DerivedStatus(); got != StatusInProgress {
		t.Errorf("Phase 4 = %q, want in_progress", got)
	}
	// Floor counts — catches mass-deletion regressions without pinning exact values.
	if n := len(p.Phases); n < 6 {
		t.Errorf("phase count = %d, want >= 6", n)
	}
	s := p.Stats()
	if s.Subphases.Total < 50 {
		t.Errorf("subphase total = %d, want >= 50", s.Subphases.Total)
	}
	if s.Items.Total < 100 {
		t.Errorf("item total = %d, want >= 100", s.Items.Total)
	}
}

func TestLoad_RealFile_ContractMetadata(t *testing.T) {
	p, err := Load("../../docs/content/building-gormes/architecture_plan/progress.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := Validate(p); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	cases := []struct {
		phase    string
		subphase string
		item     string
		contract string
		trust    string
	}{
		{"1", "1.C", "Orchestrator failure-row stabilization for 4-8 workers", "Worker verification and failure-taxonomy contract", "system"},
		{"1", "1.C", "Soft-success-nonzero bats coverage", "Soft-success nonzero recovery guard", "operator"},
		{"2", "2.F.5", "Steer slash command registry + queue fallback", "Registry-owned active-turn steering command", "gateway"},
		{"3", "3.E.7", "Cross-chat deny-path fixtures", "Same-chat default recall with explicit user-scope widening", "system"},
		{"4", "4.A", "Provider interface + stream fixture harness", "Provider-neutral request and stream event transcript harness", "system"},
		{"4", "4.A", "Tool-call normalization + continuation contract", "Cross-provider tool-call continuation contract", "system"},
		{"4", "4.B", "ContextEngine interface + status tool contract", "Stable context engine status and compression boundary", "operator"},
		{"4", "4.H", "Provider-side resilience", "Provider resilience umbrella over retry, cache, rate, and budget behavior", "system"},
		{"4", "4.H", "Classified provider-error taxonomy", "Structured provider error classification contract", "system"},
		{"5", "5.A", "Tool registry inventory + schema parity harness", "Operation and tool descriptor parity before handler ports", "child-agent"},
		{"6", "6.C", "Portable SKILL.md format", "Reviewed skill-as-code storage format", "operator"},
	}

	for _, tc := range cases {
		items := itemsByName(p.Phases[tc.phase].Subphases[tc.subphase].Items)
		it := items[tc.item]
		if it.Contract != tc.contract {
			t.Fatalf("%s contract = %q, want %q", tc.item, it.Contract, tc.contract)
		}
		if it.DegradedMode == "" || it.Fixture == "" || len(it.SourceRefs) == 0 {
			t.Fatalf("%s missing degraded_mode, fixture, or source_refs: %+v", tc.item, it)
		}
		if it.ContractStatus == "" || len(it.Acceptance) == 0 {
			t.Fatalf("%s missing contract_status or acceptance: %+v", tc.item, it)
		}
		if !containsString(it.TrustClass, tc.trust) {
			t.Fatalf("%s trust_class = %v, want %q", tc.item, it.TrustClass, tc.trust)
		}
	}
}

func TestLoad_RealFile_Phase1PlannerWrapperCloseout(t *testing.T) {
	p, err := Load("../../docs/content/building-gormes/architecture_plan/progress.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := Validate(p); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	items := itemsByName(p.Phases["1"].Subphases["1.C"].Items)
	planner := items["Planner wrapper/test consistency closeout"]
	if planner.Status != StatusComplete {
		t.Fatalf("Planner wrapper closeout status = %q, want complete", planner.Status)
	}
	if planner.Contract != "Planner wrapper compatibility contract" || planner.ContractStatus != ContractStatusValidated {
		t.Fatalf("Planner wrapper closeout contract = %q/%q, want validated compatibility contract", planner.Contract, planner.ContractStatus)
	}
	for _, want := range []string{
		"scripts/gormes-architecture-task-manager.sh",
		"scripts/architectureplanneragent.sh",
	} {
		if !containsString(planner.WriteScope, want) {
			t.Fatalf("Planner wrapper closeout write_scope = %v, want %q", planner.WriteScope, want)
		}
	}
	if !containsString(planner.TestCommands, "go test ./internal -run ArchitecturePlanner -count=1") {
		t.Fatalf("Planner wrapper closeout test_commands = %v, want focused ArchitecturePlanner test", planner.TestCommands)
	}
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func TestLoad_RealFile_Phase4Anthropic(t *testing.T) {
	p, err := Load("../../docs/content/building-gormes/architecture_plan/progress.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	adapters := p.Phases["4"].Subphases["4.A"]
	if got := adapters.DerivedStatus(); got != StatusInProgress {
		t.Fatalf("Phase 4.A = %q, want in_progress", got)
	}
	items := itemsByName(adapters.Items)
	anthropic := items["Anthropic"]
	if anthropic.Status != StatusComplete {
		t.Fatalf("Phase 4.A Anthropic status = %q, want complete", anthropic.Status)
	}
	if !strings.Contains(anthropic.Note, "cache-control metadata") || !strings.Contains(anthropic.Note, "rate-limit fixtures") {
		t.Fatalf("Phase 4.A Anthropic note = %q, want landed adapter detail", anthropic.Note)
	}
}

func TestLoad_RealFile_Phase2Ledger(t *testing.T) {
	p, err := Load("../../docs/content/building-gormes/architecture_plan/progress.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cron := p.Phases["2"].Subphases["2.D"]
	if got := cron.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 2.D = %q, want complete", got)
	}
	cronItems := itemStatusByName(cron.Items)
	for name, want := range map[string]Status{
		"robfig/cron scheduler + bbolt job store":          StatusComplete,
		"SQLite cron_runs audit + CRON.md mirror":          StatusComplete,
		"Heartbeat [SYSTEM:] + [SILENT] delivery contract": StatusComplete,
	} {
		if got := cronItems[name]; got != want {
			t.Errorf("Phase 2.D item %q = %q, want %q", name, got, want)
		}
	}

	runtimeCore := p.Phases["2"].Subphases["2.E.0"]
	if got := runtimeCore.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 2.E.0 = %q, want complete", got)
	}
	runtimeNext := p.Phases["2"].Subphases["2.E.1"]
	if got := runtimeNext.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 2.E.1 = %q, want complete", got)
	}
	runtimeNextItems := itemStatusByName(runtimeNext.Items)
	for name, want := range map[string]Status{
		"Runner-enforced tool allowlists + blocked-tool policy": StatusComplete,
		"Tool-call audit in typed child results":                StatusComplete,
		"Real child Hermes stream loop":                         StatusComplete,
		"GBrain minion-orchestrator routing policy":             StatusComplete,
		"Durable subagent/job ledger":                           StatusComplete,
	} {
		if got := runtimeNextItems[name]; got != want {
			t.Errorf("Phase 2.E.1 item %q = %q, want %q", name, got, want)
		}
	}

	gateway := p.Phases["2"].Subphases["2.B.2"]
	if got := gateway.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 2.B.2 = %q, want complete", got)
	}
	gatewayItems := itemStatusByName(gateway.Items)
	for name, want := range map[string]Status{
		"Reusable gateway chassis":                StatusComplete,
		"Telegram on shared chassis":              StatusComplete,
		"gormes gateway multi-channel entrypoint": StatusComplete,
		"Discord": StatusComplete,
	} {
		if got := gatewayItems[name]; got != want {
			t.Errorf("Phase 2.B.2 item %q = %q, want %q", name, got, want)
		}
	}

	slack := p.Phases["2"].Subphases["2.B.3"]
	if got := slack.DerivedStatus(); got != StatusInProgress {
		t.Fatalf("Phase 2.B.3 = %q, want in_progress", got)
	}
	slackItems := itemStatusByName(slack.Items)
	for name, want := range map[string]Status{
		"Slack Socket Mode adapter":                      StatusComplete,
		"Thread routing + coalesced reply flow":          StatusComplete,
		"Slack CommandRegistry parser wiring":            StatusPlanned,
		"Slack gateway.Channel adapter shim":             StatusPlanned,
		"Slack config + cmd/gormes gateway registration": StatusPlanned,
	} {
		if got := slackItems[name]; got != want {
			t.Errorf("Phase 2.B.3 item %q = %q, want %q", name, got, want)
		}
	}

	skills := p.Phases["2"].Subphases["2.G"]
	if got := skills.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 2.G = %q, want complete", got)
	}
	skillItems := itemStatusByName(skills.Items)
	for name, want := range map[string]Status{
		"SKILL.md parsing + active store":        StatusComplete,
		"Deterministic selection + prompt block": StatusComplete,
		"Kernel injection + usage log":           StatusComplete,
		"Inactive candidate drafting":            StatusComplete,
		"Explicit promotion flow":                StatusComplete,
	} {
		if got := skillItems[name]; got != want {
			t.Errorf("Phase 2.G item %q = %q, want %q", name, got, want)
		}
	}
}

func TestLoad_RealFile_Phase2ExecutionQueue(t *testing.T) {
	p, err := Load("../../docs/content/building-gormes/architecture_plan/progress.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	e0 := p.Phases["2"].Subphases["2.E.0"]
	if e0.Priority != "P0" {
		t.Fatalf("Phase 2.E.0 priority = %q, want P0", e0.Priority)
	}
	if got := e0.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 2.E.0 = %q, want complete", got)
	}

	e1 := p.Phases["2"].Subphases["2.E.1"]
	if e1.Priority != "P0" {
		t.Fatalf("Phase 2.E.1 priority = %q, want P0", e1.Priority)
	}
	if got := e1.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 2.E.1 = %q, want complete", got)
	}
	e1Items := itemsByName(e1.Items)
	policy := e1Items["Runner-enforced tool allowlists + blocked-tool policy"]
	if policy.Status != StatusComplete {
		t.Fatalf("Phase 2.E.1 policy status = %q, want complete", policy.Status)
	}
	if !strings.Contains(policy.Note, "TDD") {
		t.Fatalf("Phase 2.E.1 policy note = %q, want TDD guidance", policy.Note)
	}
	audit := e1Items["Tool-call audit in typed child results"]
	if audit.Status != StatusComplete {
		t.Fatalf("Phase 2.E.1 tool-call audit status = %q, want complete", audit.Status)
	}
	if !strings.Contains(audit.Note, "TDD") {
		t.Fatalf("Phase 2.E.1 tool-call audit note = %q, want TDD guidance", audit.Note)
	}
	child := e1Items["Real child Hermes stream loop"]
	if child.Status != StatusComplete {
		t.Fatalf("Phase 2.E.1 child runner status = %q, want complete", child.Status)
	}
	if !strings.Contains(child.Note, "HermesRunner") {
		t.Fatalf("Phase 2.E.1 child runner note = %q, want HermesRunner implementation detail", child.Note)
	}
	minionPolicy := e1Items["GBrain minion-orchestrator routing policy"]
	if minionPolicy.Status != StatusComplete {
		t.Fatalf("Phase 2.E.1 minion policy status = %q, want complete", minionPolicy.Status)
	}
	if minionPolicy.ContractStatus != ContractStatusValidated || minionPolicy.SliceSize != SliceSizeSmall || minionPolicy.ExecutionOwner != ExecutionOwnerOrchestrator {
		t.Fatalf("Phase 2.E.1 minion policy metadata = status %q size %q owner %q, want validated/small/orchestrator", minionPolicy.ContractStatus, minionPolicy.SliceSize, minionPolicy.ExecutionOwner)
	}
	if !containsString(minionPolicy.SourceRefs, "../gbrain/skills/minion-orchestrator/SKILL.md") || !containsString(minionPolicy.Unblocks, "Durable subagent/job ledger") {
		t.Fatalf("Phase 2.E.1 minion policy refs/unblocks = refs %v unblocks %v, want GBrain skill ref and durable ledger unblock", minionPolicy.SourceRefs, minionPolicy.Unblocks)
	}
	durableLedger := e1Items["Durable subagent/job ledger"]
	if durableLedger.Status != StatusComplete {
		t.Fatalf("Phase 2.E.1 durable ledger status = %q, want complete", durableLedger.Status)
	}
	if durableLedger.ContractStatus != ContractStatusValidated || !strings.Contains(durableLedger.Note, "SQLite-first durable ledger") {
		t.Fatalf("Phase 2.E.1 durable ledger metadata = contract_status %q note %q, want validated SQLite-first ledger", durableLedger.ContractStatus, durableLedger.Note)
	}

	whatsApp := p.Phases["2"].Subphases["2.B.4"]
	if whatsApp.Priority != "P1" {
		t.Fatalf("Phase 2.B.4 priority = %q, want P1", whatsApp.Priority)
	}
	if got := whatsApp.DerivedStatus(); got != StatusInProgress {
		t.Fatalf("Phase 2.B.4 = %q, want in_progress", got)
	}
	whatsAppItems := itemsByName(whatsApp.Items)
	decision := whatsAppItems["Bridge-vs-native runtime decision"]
	if decision.Status != StatusComplete {
		t.Fatalf("Phase 2.B.4 decision status = %q, want complete", decision.Status)
	}
	if !strings.Contains(decision.Note, "gateway/platforms/whatsapp.py") || !strings.Contains(decision.Note, "DecideRuntime") || !strings.Contains(decision.Note, "native-first") {
		t.Fatalf("Phase 2.B.4 decision note = %q, want upstream-whatsapp/DecideRuntime/native-first detail", decision.Note)
	}
	inbound := whatsAppItems["Inbound normalization + command passthrough"]
	if inbound.Status != StatusComplete {
		t.Fatalf("Phase 2.B.4 inbound normalization status = %q, want complete", inbound.Status)
	}
	if !strings.Contains(inbound.Note, "NormalizeInbound") || !strings.Contains(inbound.Note, "ParseInboundText") {
		t.Fatalf("Phase 2.B.4 inbound normalization note = %q, want NormalizeInbound/ParseInboundText detail", inbound.Note)
	}
	whatsAppPairing := whatsAppItems["Pairing, reconnect, and send contract"]
	if whatsAppPairing.Status != StatusPlanned {
		t.Fatalf("Phase 2.B.4 pairing/reconnect/send status = %q, want planned", whatsAppPairing.Status)
	}
	if !strings.Contains(whatsAppPairing.Note, "pairing state") || !strings.Contains(whatsAppPairing.Note, "reconnects") {
		t.Fatalf("Phase 2.B.4 pairing/reconnect/send note = %q, want pairing-state/reconnect detail", whatsAppPairing.Note)
	}

	weChat := p.Phases["2"].Subphases["2.B.10"]
	if weChat.Priority != "P1" {
		t.Fatalf("Phase 2.B.10 priority = %q, want P1", weChat.Priority)
	}
	if got := weChat.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 2.B.10 = %q, want complete", got)
	}
	weChatItems := itemsByName(weChat.Items)
	weComWeiXin := weChatItems["WeCom + WeiXin shared-chassis bot seam"]
	if weComWeiXin.Status != StatusComplete {
		t.Fatalf("Phase 2.B.10 WeCom + WeiXin shared-chassis bot seam status = %q, want complete", weComWeiXin.Status)
	}
	if !strings.Contains(weComWeiXin.Note, "internal/channels/wecom") || !strings.Contains(weComWeiXin.Note, "internal/channels/weixin") {
		t.Fatalf("Phase 2.B.10 WeCom + WeiXin shared-chassis bot seam note = %q, want WeCom/WeiXin detail", weComWeiXin.Note)
	}
	weComTransport := weChatItems["WeCom + WeiXin transport/bootstrap layer"]
	if weComTransport.Status != StatusComplete {
		t.Fatalf("Phase 2.B.10 WeCom + WeiXin transport/bootstrap status = %q, want complete", weComTransport.Status)
	}
	if !strings.Contains(weComTransport.Note, "runtime.go") || !strings.Contains(weComTransport.Note, "context-token") {
		t.Fatalf("Phase 2.B.10 WeCom + WeiXin transport/bootstrap note = %q, want runtime/context-token detail", weComTransport.Note)
	}

	signal := p.Phases["7"].Subphases["7.A"]
	if signal.Priority != "P2" {
		t.Fatalf("Phase 7.A priority = %q, want P2", signal.Priority)
	}
	if got := signal.DerivedStatus(); got != StatusInProgress {
		t.Fatalf("Phase 7.A = %q, want in_progress", got)
	}
	signalItems := itemsByName(signal.Items)
	identity := signalItems["Inbound event normalization + session identity"]
	if identity.Status != StatusComplete {
		t.Fatalf("Phase 7.A inbound normalization status = %q, want complete", identity.Status)
	}
	if !strings.Contains(identity.Note, "NormalizeInbound") || !strings.Contains(identity.Note, "phone/UUID") {
		t.Fatalf("Phase 7.A inbound normalization note = %q, want NormalizeInbound/phone-UUID detail", identity.Note)
	}
	replySend := signalItems["Reply/send contract on shared chassis"]
	if replySend.Status != StatusComplete {
		t.Fatalf("Phase 7.A reply/send status = %q, want complete", replySend.Status)
	}
	if !strings.Contains(replySend.Note, "signal.Bot") || !strings.Contains(replySend.Note, "native group IDs") {
		t.Fatalf("Phase 7.A reply/send note = %q, want signal.Bot/native group IDs detail", replySend.Note)
	}
	transport := signalItems["Signal transport/bootstrap layer"]
	if transport.Status != StatusPlanned {
		t.Fatalf("Phase 7.A transport/bootstrap status = %q, want planned", transport.Status)
	}
	if !strings.Contains(transport.Note, "signal-cli") || !strings.Contains(transport.Note, "bridge client lifecycle") {
		t.Fatalf("Phase 7.A transport/bootstrap note = %q, want signal-cli/bridge detail", transport.Note)
	}

	routing := p.Phases["2"].Subphases["2.B.5"]
	if routing.Priority != "P1" {
		t.Fatalf("Phase 2.B.5 priority = %q, want P1", routing.Priority)
	}
	if got := routing.DerivedStatus(); got != StatusInProgress {
		t.Fatalf("Phase 2.B.5 = %q, want in_progress", got)
	}
	routingItems := itemsByName(routing.Items)
	sessionStore := routingItems["Gateway session store + SessionSource parity"]
	if sessionStore.Status != StatusComplete {
		t.Fatalf("Phase 2.B.5 session store status = %q, want complete", sessionStore.Status)
	}
	if !strings.Contains(sessionStore.Note, "TDD landed") {
		t.Fatalf("Phase 2.B.5 session store note = %q, want TDD landed guidance", sessionStore.Note)
	}
	sessionContext := routingItems["SessionContext prompt injection"]
	if sessionContext.Status != StatusComplete {
		t.Fatalf("Phase 2.B.5 session context status = %q, want complete", sessionContext.Status)
	}
	delivery := routingItems["DeliveryRouter + --deliver target parsing"]
	if delivery.Status != StatusComplete {
		t.Fatalf("Phase 2.B.5 delivery parsing status = %q, want complete", delivery.Status)
	}
	streamFanout := routingItems["Gateway stream consumer for agent-event fan-out"]
	if streamFanout.Status != StatusComplete {
		t.Fatalf("Phase 2.B.5 stream fanout status = %q, want complete", streamFanout.Status)
	}
	bluePrompt := routingItems["BlueBubbles iMessage session-context prompt guidance"]
	if bluePrompt.Status != StatusPlanned {
		t.Fatalf("Phase 2.B.5 BlueBubbles prompt guidance status = %q, want planned", bluePrompt.Status)
	}
	if bluePrompt.ContractStatus != ContractStatusFixtureReady || !containsString(bluePrompt.BlockedBy, "BlueBubbles iMessage bubble formatting parity") {
		t.Fatalf("Phase 2.B.5 BlueBubbles prompt guidance metadata = contract_status %q blocked_by %v, want fixture_ready blocked by formatter", bluePrompt.ContractStatus, bluePrompt.BlockedBy)
	}
	nonEditableFallback := routingItems["Non-editable gateway progress/commentary send fallback"]
	if nonEditableFallback.Status != StatusComplete {
		t.Fatalf("Phase 2.B.5 non-editable fallback status = %q, want complete", nonEditableFallback.Status)
	}
	if nonEditableFallback.ContractStatus != ContractStatusValidated || !containsString(nonEditableFallback.Unblocks, "BlueBubbles iMessage session-context prompt guidance") {
		t.Fatalf("Phase 2.B.5 non-editable fallback metadata = contract_status %q unblocks %v, want validated unblocking BlueBubbles prompt guidance", nonEditableFallback.ContractStatus, nonEditableFallback.Unblocks)
	}

	hooks := p.Phases["2"].Subphases["2.F.1"]
	if hooks.Priority != "P1" {
		t.Fatalf("Phase 2.F.1 priority = %q, want P1", hooks.Priority)
	}
	if got := hooks.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 2.F.1 = %q, want complete", got)
	}
	hookItemsF1 := itemsByName(hooks.Items)
	commandRegistry := hookItemsF1["Canonical CommandDef registry"]
	if commandRegistry.Status != StatusComplete {
		t.Fatalf("Phase 2.F.1 command registry status = %q, want complete", commandRegistry.Status)
	}
	if !strings.Contains(commandRegistry.Note, "ResolveCommand") {
		t.Fatalf("Phase 2.F.1 command registry note = %q, want ResolveCommand detail", commandRegistry.Note)
	}
	dispatch := hookItemsF1["Gateway slash dispatch + per-platform exposure"]
	if dispatch.Status != StatusComplete {
		t.Fatalf("Phase 2.F.1 dispatch status = %q, want complete", dispatch.Status)
	}
	if !strings.Contains(dispatch.Note, "Telegram") || !strings.Contains(dispatch.Note, "Slack") {
		t.Fatalf("Phase 2.F.1 dispatch note = %q, want Telegram/Slack detail", dispatch.Note)
	}

	hookRegistry := p.Phases["2"].Subphases["2.F.2"]
	if hookRegistry.Priority != "P2" {
		t.Fatalf("Phase 2.F.2 priority = %q, want P2", hookRegistry.Priority)
	}
	if got := hookRegistry.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 2.F.2 = %q, want complete", got)
	}
	hookItems := itemsByName(hookRegistry.Items)
	managerHooks := hookItems["Gateway per-event hook registry"]
	if managerHooks.Status != StatusComplete {
		t.Fatalf("Phase 2.F.2 gateway hook registry status = %q, want complete", managerHooks.Status)
	}
	if !strings.Contains(managerHooks.Note, "TDD") {
		t.Fatalf("Phase 2.F.2 gateway hook registry note = %q, want TDD guidance", managerHooks.Note)
	}
	boot := hookItems["Built-in BOOT.md startup hook"]
	if boot.Status != StatusComplete {
		t.Fatalf("Phase 2.F.2 BOOT hook status = %q, want complete", boot.Status)
	}

	lifecycle := p.Phases["2"].Subphases["2.F.3"]
	if lifecycle.Priority != "P2" {
		t.Fatalf("Phase 2.F.3 priority = %q, want P2", lifecycle.Priority)
	}
	if got := lifecycle.DerivedStatus(); got != StatusInProgress {
		t.Fatalf("Phase 2.F.3 = %q, want in_progress", got)
	}
	lifecycleItems := itemsByName(lifecycle.Items)
	drain := lifecycleItems["Graceful restart drain + managed shutdown"]
	if drain.Status != StatusComplete {
		t.Fatalf("Phase 2.F.3 drain status = %q, want complete", drain.Status)
	}
	if !strings.Contains(drain.Note, "TDD") {
		t.Fatalf("Phase 2.F.3 drain note = %q, want TDD guidance", drain.Note)
	}
	startupCleanup := lifecycleItems["Adapter startup failure cleanup contract"]
	if startupCleanup.Status != StatusComplete {
		t.Fatalf("Phase 2.F.3 startup cleanup status = %q, want complete", startupCleanup.Status)
	}
	if !strings.Contains(startupCleanup.Note, "Manager.Run") ||
		!strings.Contains(startupCleanup.Note, "Disconnect") ||
		!strings.Contains(startupCleanup.Note, "Discord") {
		t.Fatalf("Phase 2.F.3 startup cleanup note = %q, want Manager.Run/Disconnect/Discord detail", startupCleanup.Note)
	}
	operator := p.Phases["2"].Subphases["2.F.4"]
	if operator.Priority != "P3" {
		t.Fatalf("Phase 2.F.4 priority = %q, want P3", operator.Priority)
	}

	mail := p.Phases["7"].Subphases["7.B"]
	if mail.Priority != "P3" {
		t.Fatalf("Phase 7.B priority = %q, want P3", mail.Priority)
	}
	if got := mail.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 7.B = %q, want complete", got)
	}
	mailItems := itemsByName(mail.Items)
	email := mailItems["Email ingress + outbound delivery contract"]
	if email.Status != StatusComplete {
		t.Fatalf("Phase 7.B email status = %q, want complete", email.Status)
	}
	if !strings.Contains(email.Note, "TDD landed") {
		t.Fatalf("Phase 7.B email note = %q, want TDD landed guidance", email.Note)
	}
	sms := mailItems["SMS ingress + outbound delivery contract"]
	if sms.Status != StatusComplete {
		t.Fatalf("Phase 7.B sms status = %q, want complete", sms.Status)
	}
	if !strings.Contains(sms.Note, "NormalizeInbound") || !strings.Contains(sms.Note, "BuildDelivery") {
		t.Fatalf("Phase 7.B sms note = %q, want NormalizeInbound/BuildDelivery detail", sms.Note)
	}

	webhookIngress := p.Phases["7"].Subphases["7.D"]
	if webhookIngress.Priority != "P4" {
		t.Fatalf("Phase 7.D priority = %q, want P4", webhookIngress.Priority)
	}
	if got := webhookIngress.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 7.D = %q, want complete", got)
	}
	webhookItems := itemsByName(webhookIngress.Items)
	signed := webhookItems["Signed event parsing + auth gates"]
	if signed.Status != StatusComplete {
		t.Fatalf("Phase 7.D signed-ingress status = %q, want complete", signed.Status)
	}
	if !strings.Contains(signed.Note, "ParseInbound") || !strings.Contains(signed.Note, "ValidateSignature") {
		t.Fatalf("Phase 7.D signed-ingress note = %q, want ParseInbound/ValidateSignature detail", signed.Note)
	}
	promptBridge := webhookItems["Prompt-to-delivery routing bridge"]
	if promptBridge.Status != StatusComplete {
		t.Fatalf("Phase 7.D prompt bridge status = %q, want complete", promptBridge.Status)
	}

	matrixMattermost := p.Phases["7"].Subphases["7.C"]
	if matrixMattermost.Priority != "P4" {
		t.Fatalf("Phase 7.C priority = %q, want P4", matrixMattermost.Priority)
	}
	if got := matrixMattermost.DerivedStatus(); got != StatusInProgress {
		t.Fatalf("Phase 7.C = %q, want in_progress", got)
	}
	matrixItems := itemsByName(matrixMattermost.Items)
	threadedText := matrixItems["Threaded text adapter contract suite"]
	if threadedText.Status != StatusComplete {
		t.Fatalf("Phase 7.C threaded text status = %q, want complete", threadedText.Status)
	}
	matrixBot := matrixItems["Matrix shared-chassis bot seam"]
	if matrixBot.Status != StatusPlanned {
		t.Fatalf("Phase 7.C matrix bot status = %q, want planned", matrixBot.Status)
	}
	if !strings.Contains(matrixBot.Note, "internal/channels/threadtext") || !strings.Contains(matrixBot.Note, "thread") {
		t.Fatalf("Phase 7.C matrix bot note = %q, want threadtext/thread detail", matrixBot.Note)
	}
	mattermostBot := matrixItems["Mattermost shared-chassis bot seam"]
	if mattermostBot.Status != StatusPlanned {
		t.Fatalf("Phase 7.C mattermost bot status = %q, want planned", mattermostBot.Status)
	}
	if !strings.Contains(mattermostBot.Note, "internal/channels/threadtext") || !strings.Contains(mattermostBot.Note, "REST/WS") {
		t.Fatalf("Phase 7.C mattermost bot note = %q, want threadtext/REST-WS detail", mattermostBot.Note)
	}
	matrixBootstrap := matrixItems["Matrix real client/bootstrap layer"]
	if matrixBootstrap.Status != StatusPlanned {
		t.Fatalf("Phase 7.C matrix bootstrap status = %q, want planned", matrixBootstrap.Status)
	}
	mattermostBootstrap := matrixItems["Mattermost REST/WS bootstrap layer"]
	if mattermostBootstrap.Status != StatusPlanned {
		t.Fatalf("Phase 7.C mattermost bootstrap status = %q, want planned", mattermostBootstrap.Status)
	}

	longTail := p.Phases["7"].Subphases["7.E"]
	if longTail.Priority != "P4" {
		t.Fatalf("Phase 7.E priority = %q, want P4", longTail.Priority)
	}
	if got := longTail.DerivedStatus(); got != StatusInProgress {
		t.Fatalf("Phase 7.E = %q, want in_progress", got)
	}
	longTailItems := itemsByName(longTail.Items)
	blueBubblesHA := longTailItems["BlueBubbles + HomeAssistant adapters"]
	if blueBubblesHA.Status != StatusComplete {
		t.Fatalf("Phase 7.E BlueBubbles + HomeAssistant adapters status = %q, want complete", blueBubblesHA.Status)
	}
	if !strings.Contains(blueBubblesHA.Note, "internal/channels/bluebubbles") || !strings.Contains(blueBubblesHA.Note, "internal/channels/homeassistant") {
		t.Fatalf("Phase 7.E BlueBubbles + HomeAssistant adapters note = %q, want BlueBubbles/HomeAssistant contract detail", blueBubblesHA.Note)
	}
	blueBubblesParity := longTailItems["BlueBubbles iMessage bubble formatting parity"]
	if blueBubblesParity.Status != StatusPlanned {
		t.Fatalf("Phase 2.B.10 BlueBubbles iMessage parity status = %q, want planned", blueBubblesParity.Status)
	}
	if blueBubblesParity.ContractStatus != ContractStatusFixtureReady || !containsString(blueBubblesParity.Unblocks, "BlueBubbles iMessage session-context prompt guidance") {
		t.Fatalf("Phase 2.B.10 BlueBubbles iMessage parity metadata = contract_status %q unblocks %v, want fixture_ready unblocking prompt guidance", blueBubblesParity.ContractStatus, blueBubblesParity.Unblocks)
	}
	feishu := longTailItems["Feishu shared-chassis bot seam"]
	if feishu.Status != StatusComplete {
		t.Fatalf("Phase 7.E Feishu shared-chassis bot seam status = %q, want complete", feishu.Status)
	}
	if !strings.Contains(feishu.Note, "internal/channels/feishu") {
		t.Fatalf("Phase 7.E Feishu shared-chassis bot seam note = %q, want Feishu detail", feishu.Note)
	}
	dingTalk := longTailItems["DingTalk shared-chassis bot seam"]
	if dingTalk.Status != StatusComplete {
		t.Fatalf("Phase 7.E DingTalk shared-chassis bot seam status = %q, want complete", dingTalk.Status)
	}
	if !strings.Contains(dingTalk.Note, "internal/channels/dingtalk") {
		t.Fatalf("Phase 7.E DingTalk shared-chassis bot seam note = %q, want DingTalk detail", dingTalk.Note)
	}
	qqBot := longTailItems["QQ Bot shared-chassis bot seam"]
	if qqBot.Status != StatusComplete {
		t.Fatalf("Phase 7.E QQ Bot shared-chassis bot seam status = %q, want complete", qqBot.Status)
	}
	if !strings.Contains(qqBot.Note, "internal/channels/qqbot") {
		t.Fatalf("Phase 7.E QQ Bot shared-chassis bot seam note = %q, want QQ detail", qqBot.Note)
	}
	feishuTransport := longTailItems["Feishu transport/bootstrap layer"]
	if feishuTransport.Status != StatusPlanned {
		t.Fatalf("Phase 7.E Feishu transport/bootstrap status = %q, want planned", feishuTransport.Status)
	}
	dingTalkTransport := longTailItems["DingTalk transport/bootstrap layer"]
	if dingTalkTransport.Status != StatusComplete {
		t.Fatalf("Phase 7.E DingTalk transport/bootstrap status = %q, want complete", dingTalkTransport.Status)
	}
	if !strings.Contains(dingTalkTransport.Note, "DecideRuntime") || !strings.Contains(dingTalkTransport.Note, "ReplySender") {
		t.Fatalf("Phase 7.E DingTalk transport/bootstrap note = %q, want DecideRuntime/ReplySender detail", dingTalkTransport.Note)
	}
	dingTalkSDK := longTailItems["DingTalk real SDK binding"]
	if dingTalkSDK.Status != StatusPlanned {
		t.Fatalf("Phase 7.E DingTalk real SDK binding status = %q, want planned", dingTalkSDK.Status)
	}
	if !strings.Contains(dingTalkSDK.Note, "real DingTalk SDK") {
		t.Fatalf("Phase 7.E DingTalk real SDK binding note = %q, want real SDK detail", dingTalkSDK.Note)
	}
	dingTalkCards := longTailItems["DingTalk AI Cards streaming-update contract"]
	if dingTalkCards.Status != StatusComplete {
		t.Fatalf("Phase 7.E DingTalk AI Cards streaming-update contract status = %q, want complete", dingTalkCards.Status)
	}
	if !strings.Contains(dingTalkCards.Note, "AICardBot") || !strings.Contains(dingTalkCards.Note, "FinalizingMessageEditor") {
		t.Fatalf("Phase 7.E DingTalk AI Cards streaming-update note = %q, want AICardBot/FinalizingMessageEditor detail", dingTalkCards.Note)
	}
	dingTalkReactions := longTailItems["DingTalk emoji reaction send/receive parity"]
	if dingTalkReactions.Status != StatusComplete {
		t.Fatalf("Phase 7.E DingTalk emoji reaction send/receive parity status = %q, want complete", dingTalkReactions.Status)
	}
	if !strings.Contains(dingTalkReactions.Note, "EmojiReactionClient") || !strings.Contains(dingTalkReactions.Note, "Thinking") || !strings.Contains(dingTalkReactions.Note, "Done") {
		t.Fatalf("Phase 7.E DingTalk emoji reaction note = %q, want EmojiReactionClient/Thinking/Done detail", dingTalkReactions.Note)
	}
	qqTransport := longTailItems["QQ Bot transport/bootstrap layer"]
	if qqTransport.Status != StatusPlanned {
		t.Fatalf("Phase 7.E QQ Bot transport/bootstrap status = %q, want planned", qqTransport.Status)
	}

	lifecycleStore := lifecycleItems["Pairing read-model schema + atomic persistence"]
	if lifecycleStore.Status != StatusPlanned {
		t.Fatalf("Phase 2.F.3 pairing read model status = %q, want planned", lifecycleStore.Status)
	}
	if !strings.Contains(lifecycleStore.Note, "gateway/pairing.py") || !strings.Contains(lifecycleStore.Note, "pairing.json") {
		t.Fatalf("Phase 2.F.3 pairing read model note = %q, want pairing-donor/pairing.json detail", lifecycleStore.Note)
	}
	approval := lifecycleItems["Pairing approval + rate-limit semantics"]
	if approval.Status != StatusPlanned {
		t.Fatalf("Phase 2.F.3 pairing approval status = %q, want planned", approval.Status)
	}
	if !strings.Contains(approval.Note, "rate limiting") || !strings.Contains(approval.Note, "lockout") {
		t.Fatalf("Phase 2.F.3 pairing approval note = %q, want rate-limit/lockout detail", approval.Note)
	}
	statusReadout := lifecycleItems["`gormes gateway status` read-only command"]
	if statusReadout.Status != StatusPlanned {
		t.Fatalf("Phase 2.F.3 gateway status readout status = %q, want planned", statusReadout.Status)
	}
	if !strings.Contains(statusReadout.Note, "gormes gateway status") || !strings.Contains(statusReadout.Note, "configured channels") {
		t.Fatalf("Phase 2.F.3 gateway status readout note = %q, want command/configured-channels detail", statusReadout.Note)
	}
	statusJSON := lifecycleItems["Runtime status JSON + PID/process validation"]
	if statusJSON.Status != StatusPlanned {
		t.Fatalf("Phase 2.F.3 runtime status JSON status = %q, want planned", statusJSON.Status)
	}
	tokenLocks := lifecycleItems["Token-scoped gateway locks"]
	if tokenLocks.Status != StatusPlanned {
		t.Fatalf("Phase 2.F.3 token-scoped locks status = %q, want planned", tokenLocks.Status)
	}
	if !strings.Contains(tokenLocks.Note, "acquire_scoped_lock") || !strings.Contains(tokenLocks.Note, "credential hash") {
		t.Fatalf("Phase 2.F.3 token-scoped locks note = %q, want upstream lock/credential-hash detail", tokenLocks.Note)
	}
	restartMarkers := lifecycleItems["Gateway /restart command + takeover markers"]
	if restartMarkers.Status != StatusPlanned {
		t.Fatalf("Phase 2.F.3 restart markers status = %q, want planned", restartMarkers.Status)
	}
	if !strings.Contains(restartMarkers.Note, "/restart") || !strings.Contains(restartMarkers.Note, "takeover-marker") {
		t.Fatalf("Phase 2.F.3 restart markers note = %q, want restart/takeover-marker detail", restartMarkers.Note)
	}
	lifecycleWriters := lifecycleItems["Channel lifecycle writers into status model"]
	if lifecycleWriters.Status != StatusComplete {
		t.Fatalf("Phase 2.F.3 lifecycle writers status = %q, want complete", lifecycleWriters.Status)
	}
	if !strings.Contains(lifecycleWriters.Note, "RuntimeStatusStore") || !strings.Contains(lifecycleWriters.Note, "gateway_state.json") {
		t.Fatalf("Phase 2.F.3 lifecycle writers note = %q, want runtime status store evidence", lifecycleWriters.Note)
	}

	if got := operator.DerivedStatus(); got != StatusPlanned {
		t.Fatalf("Phase 2.F.4 = %q, want planned", got)
	}
	operatorItems := itemsByName(operator.Items)
	homeRules := operatorItems["Home channel ownership rules"]
	if homeRules.Status != StatusPlanned {
		t.Fatalf("Phase 2.F.4 home channel ownership status = %q, want planned", homeRules.Status)
	}
	notifyRoute := operatorItems["Notify-to delivery routing"]
	if notifyRoute.Status != StatusPlanned {
		t.Fatalf("Phase 2.F.4 notify-to routing status = %q, want planned", notifyRoute.Status)
	}
	directory := operatorItems["Channel directory atomic persistence + lookup"]
	if directory.Status != StatusPlanned {
		t.Fatalf("Phase 2.F.4 channel directory contract status = %q, want planned", directory.Status)
	}
	if !strings.Contains(directory.Note, "gateway/channel_directory.py") || !strings.Contains(directory.Note, "channel_directory.json") {
		t.Fatalf("Phase 2.F.4 channel directory contract note = %q, want channel-directory-donor/json detail", directory.Note)
	}
	rememberSource := operatorItems["Manager remember-source hook"]
	if rememberSource.Status != StatusPlanned {
		t.Fatalf("Phase 2.F.4 manager remember-source status = %q, want planned", rememberSource.Status)
	}
}

func TestLoad_RealFile_Phase3Ledger(t *testing.T) {
	p, err := Load("../../docs/content/building-gormes/architecture_plan/progress.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	semantic := p.Phases["3"].Subphases["3.D"]
	if got := semantic.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 3.D = %q, want complete", got)
	}
	semanticItems := itemStatusByName(semantic.Items)
	for name, want := range map[string]Status{
		"Ollama embeddings":        StatusComplete,
		"Vector cache":             StatusComplete,
		"Cosine similarity recall": StatusComplete,
		"Hybrid fusion":            StatusComplete,
	} {
		if got := semanticItems[name]; got != want {
			t.Errorf("Phase 3.D item %q = %q, want %q", name, got, want)
		}
	}

	mirror := p.Phases["3"].Subphases["3.D.5"]
	if got := mirror.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 3.D.5 = %q, want complete", got)
	}

	sessionSearch := p.Phases["3"].Subphases["3.E.8"]
	if got := sessionSearch.DerivedStatus(); got != StatusInProgress {
		t.Fatalf("Phase 3.E.8 = %q, want in_progress", got)
	}
	e8Items := itemsByName(sessionSearch.Items)
	lineage := e8Items["parent_session_id lineage for compression splits"]
	if lineage.Status != StatusComplete {
		t.Fatalf("Phase 3.E.8 lineage status = %q, want complete", lineage.Status)
	}
	search := e8Items["Source-filtered session/message search core"]
	if search.Status != StatusComplete {
		t.Fatalf("Phase 3.E.8 source-filtered search status = %q, want complete", search.Status)
	}
	if !strings.Contains(search.Note, "SearchMessages") ||
		!strings.Contains(search.Note, "source allowlists") ||
		!strings.Contains(search.Note, "split source/chat key") {
		t.Fatalf("Phase 3.E.8 source-filtered search note = %q, want SearchMessages/source/split-key detail", search.Note)
	}
	gonchoScope := e8Items["GONCHO user-scope search/context parameters"]
	if gonchoScope.Status != StatusComplete {
		t.Fatalf("Phase 3.E.8 GONCHO scope status = %q, want complete", gonchoScope.Status)
	}
	if !strings.Contains(gonchoScope.Note, "scope=user") || !strings.Contains(gonchoScope.Note, "Honcho-compatible tool schemas") {
		t.Fatalf("Phase 3.E.8 GONCHO scope note = %q, want scope/tool-schema detail", gonchoScope.Note)
	}
}

func TestLoad_RealFile_Phase3ExecutionQueue(t *testing.T) {
	p, err := Load("../../docs/content/building-gormes/architecture_plan/progress.json")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	index := p.Phases["3"].Subphases["3.E.1"]
	if index.Priority != "P0" {
		t.Fatalf("Phase 3.E.1 priority = %q, want P0", index.Priority)
	}
	indexItems := itemsByName(index.Items)
	mirror := indexItems["Read-only bbolt sessions.db -> index.yaml mirror"]
	if mirror.Status != StatusComplete {
		t.Fatalf("Phase 3.E.1 mirror status = %q, want complete", mirror.Status)
	}
	if !strings.Contains(mirror.Note, "SessionIndexMirror") {
		t.Fatalf("Phase 3.E.1 mirror note = %q, want SessionIndexMirror implementation detail", mirror.Note)
	}
	refresh := indexItems["Deterministic mirror refresh without mutating session state"]
	if refresh.Status != StatusComplete {
		t.Fatalf("Phase 3.E.1 refresh status = %q, want complete", refresh.Status)
	}
	if !strings.Contains(refresh.Note, "background refresh loop") {
		t.Fatalf("Phase 3.E.1 refresh note = %q, want runtime-refresh detail", refresh.Note)
	}

	audit := p.Phases["3"].Subphases["3.E.2"]
	if audit.Priority != "P0" {
		t.Fatalf("Phase 3.E.2 priority = %q, want P0", audit.Priority)
	}
	if got := audit.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 3.E.2 = %q, want complete", got)
	}
	auditItems := itemsByName(audit.Items)
	writer := auditItems["Append-only JSONL writer + schema"]
	if writer.Status != StatusComplete {
		t.Fatalf("Phase 3.E.2 writer status = %q, want complete", writer.Status)
	}
	if !strings.Contains(writer.Note, "JSONL") {
		t.Fatalf("Phase 3.E.2 writer note = %q, want JSONL implementation detail", writer.Note)
	}
	hooks := auditItems["Kernel + delegate_task audit hooks"]
	if hooks.Status != StatusComplete {
		t.Fatalf("Phase 3.E.2 hooks status = %q, want complete", hooks.Status)
	}
	if !strings.Contains(hooks.Note, "delegate_task") {
		t.Fatalf("Phase 3.E.2 hooks note = %q, want delegate_task implementation detail", hooks.Note)
	}
	outcome := auditItems["Outcome, duration, and error capture"]
	if outcome.Status != StatusComplete {
		t.Fatalf("Phase 3.E.2 outcome status = %q, want complete", outcome.Status)
	}
	if !strings.Contains(outcome.Note, "duration_ms") {
		t.Fatalf("Phase 3.E.2 outcome note = %q, want duration_ms detail", outcome.Note)
	}

	status := p.Phases["3"].Subphases["3.E.4"]
	if status.Priority != "P1" {
		t.Fatalf("Phase 3.E.4 priority = %q, want P1", status.Priority)
	}
	statusItems := itemsByName(status.Items)
	command := statusItems["gormes memory status command"]
	if command.Status != StatusComplete {
		t.Fatalf("Phase 3.E.4 command status = %q, want complete", command.Status)
	}
	if !strings.Contains(command.Note, "gormes memory status") {
		t.Fatalf("Phase 3.E.4 command note = %q, want command detail", command.Note)
	}
	summary := statusItems["Extractor queue depth + dead-letter summary"]
	if summary.Status != StatusComplete {
		t.Fatalf("Phase 3.E.4 summary status = %q, want complete", summary.Status)
	}
	if !strings.Contains(summary.Note, "dead-letter error aggregation") {
		t.Fatalf("Phase 3.E.4 summary note = %q, want aggregation detail", summary.Note)
	}

	decay := p.Phases["3"].Subphases["3.E.6"]
	if decay.Priority != "P1" {
		t.Fatalf("Phase 3.E.6 priority = %q, want P1", decay.Priority)
	}
	if got := decay.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 3.E.6 = %q, want complete", got)
	}
	decayItems := itemsByName(decay.Items)
	lastSeen := decayItems["relationships.last_seen schema + backfill"]
	if lastSeen.Status != StatusComplete {
		t.Fatalf("Phase 3.E.6 last_seen status = %q, want complete", lastSeen.Status)
	}
	if !strings.Contains(lastSeen.Note, "COALESCE") {
		t.Fatalf("Phase 3.E.6 last_seen note = %q, want TDD guidance", lastSeen.Note)
	}
	writerFreshness := decayItems["Relationship writer freshness updates"]
	if writerFreshness.Status != StatusComplete {
		t.Fatalf("Phase 3.E.6 writer freshness status = %q, want complete", writerFreshness.Status)
	}
	if !strings.Contains(writerFreshness.Note, "last_seen") {
		t.Fatalf("Phase 3.E.6 writer freshness note = %q, want last_seen detail", writerFreshness.Note)
	}

	export := p.Phases["3"].Subphases["3.E.3"]
	if export.Priority != "P2" {
		t.Fatalf("Phase 3.E.3 priority = %q, want P2", export.Priority)
	}
	if got := export.DerivedStatus(); got != StatusComplete {
		t.Fatalf("Phase 3.E.3 = %q, want complete", got)
	}
	exportItems := itemStatusByName(export.Items)
	for name, want := range map[string]Status{
		"gormes session export <id> --format=markdown":         StatusComplete,
		"Render turns, tool calls, and timestamps from SQLite": StatusComplete,
	} {
		if got := exportItems[name]; got != want {
			t.Fatalf("Phase 3.E.3 item %q = %q, want %q", name, got, want)
		}
	}

	crossChat := p.Phases["3"].Subphases["3.E.7"]
	if crossChat.Priority != "P2" {
		t.Fatalf("Phase 3.E.7 priority = %q, want P2", crossChat.Priority)
	}
	if got := crossChat.DerivedStatus(); got != StatusInProgress {
		t.Fatalf("Phase 3.E.7 = %q, want in_progress", got)
	}
	crossChatItems := itemsByName(crossChat.Items)
	userID := crossChatItems["user_id concept above chat_id"]
	if userID.Status != StatusComplete {
		t.Fatalf("Phase 3.E.7 user_id status = %q, want complete", userID.Status)
	}
	if !strings.Contains(userID.Note, "internal/session") || !strings.Contains(userID.Note, "chat-to-user merges") {
		t.Fatalf("Phase 3.E.7 user_id note = %q, want session metadata + conflict detail", userID.Note)
	}
	sameChatFence := crossChatItems["Same-chat default recall fence"]
	if sameChatFence.Status != StatusComplete {
		t.Fatalf("Phase 3.E.7 same-chat fence status = %q, want complete", sameChatFence.Status)
	}
	if !strings.Contains(sameChatFence.Note, "same-chat") || !strings.Contains(sameChatFence.Note, "do not leak") {
		t.Fatalf("Phase 3.E.7 same-chat fence note = %q, want same-chat/no-leak detail", sameChatFence.Note)
	}
	userScopeRecall := crossChatItems["Opt-in user-scope recall + source filters"]
	if userScopeRecall.Status != StatusComplete {
		t.Fatalf("Phase 3.E.7 user-scope recall status = %q, want complete", userScopeRecall.Status)
	}
	if !strings.Contains(userScopeRecall.Note, "canonical user_id") ||
		!strings.Contains(userScopeRecall.Note, "source allowlists") ||
		!strings.Contains(userScopeRecall.Note, "same-chat") {
		t.Fatalf("Phase 3.E.7 user-scope recall note = %q, want user_id/source/same-chat detail", userScopeRecall.Note)
	}
	toolSchema := crossChatItems["Honcho-compatible scope/source tool schema"]
	if toolSchema.Status != StatusComplete {
		t.Fatalf("Phase 3.E.7 tool schema status = %q, want complete", toolSchema.Status)
	}
	denyFixtures := crossChatItems["Cross-chat deny-path fixtures"]
	if denyFixtures.Status != StatusComplete {
		t.Fatalf("Phase 3.E.7 deny-path fixtures status = %q, want complete", denyFixtures.Status)
	}
	if denyFixtures.ContractStatus != ContractStatusValidated {
		t.Fatalf("Phase 3.E.7 deny-path fixtures contract_status = %q, want validated", denyFixtures.ContractStatus)
	}
	operatorEvidence3E7 := crossChatItems["Cross-chat operator evidence"]
	if operatorEvidence3E7.Status != StatusPlanned {
		t.Fatalf("Phase 3.E.7 operator evidence status = %q, want planned", operatorEvidence3E7.Status)
	}

	insights := p.Phases["3"].Subphases["3.E.5"]
	if insights.Priority != "P3" {
		t.Fatalf("Phase 3.E.5 priority = %q, want P3", insights.Priority)
	}
	insightItems := itemsByName(insights.Items)
	usageWriter := insightItems["Append-only daily usage.jsonl writer"]
	if usageWriter.Status != StatusComplete {
		t.Fatalf("Phase 3.E.5 writer status = %q, want complete", usageWriter.Status)
	}
	if !strings.Contains(usageWriter.Note, "TDD landed") {
		t.Fatalf("Phase 3.E.5 writer note = %q, want TDD landed detail", usageWriter.Note)
	}
	rollups := insightItems["Session, token, and cost rollups from local runtime"]
	if rollups.Status != StatusComplete {
		t.Fatalf("Phase 3.E.5 rollups status = %q, want complete", rollups.Status)
	}
	if !strings.Contains(rollups.Note, "telemetry.Snapshot") {
		t.Fatalf("Phase 3.E.5 rollups note = %q, want telemetry.Snapshot detail", rollups.Note)
	}

	lineage := p.Phases["3"].Subphases["3.E.8"]
	if lineage.Priority != "P4" {
		t.Fatalf("Phase 3.E.8 priority = %q, want P4", lineage.Priority)
	}
	lineageItems := itemsByName(lineage.Items)
	gatewayResume := lineageItems["Gateway resume follows compression continuation"]
	if gatewayResume.Status != StatusComplete {
		t.Fatalf("Phase 3.E.8 gateway resume status = %q, want complete", gatewayResume.Status)
	}
	lineageHits := lineageItems["Lineage-aware source-filtered search hits"]
	if lineageHits.Status != StatusComplete {
		t.Fatalf("Phase 3.E.8 lineage-aware search hits status = %q, want complete", lineageHits.Status)
	}
	operatorEvidence := lineageItems["Operator-auditable search evidence"]
	if operatorEvidence.Status != StatusPlanned {
		t.Fatalf("Phase 3.E.8 operator evidence status = %q, want planned", operatorEvidence.Status)
	}
}

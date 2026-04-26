package hermes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AzureTransport names the request shape an Azure Foundry endpoint expects.
//
// Mirrors the api_mode classifications in
// hermes_cli/azure_detect.py: "chat_completions" / "anthropic_messages" /
// unknown. Constants are defined here so the path-sniff slice can reuse
// them without redefining them upstream.
type AzureTransport string

const (
	AzureTransportUnknown   AzureTransport = "unknown"
	AzureTransportOpenAI    AzureTransport = "openai_chat_completions"
	AzureTransportAnthropic AzureTransport = "anthropic_messages"
)

// AzureProbeResult is what ProbeAzureFoundry returns to the caller.
//
// Transport is the classification verdict. Models is populated only when
// the /models probe returned an OpenAI-shaped catalog (possibly empty).
// Reason is a short human summary suitable for display in the wizard.
// Evidence captures the URLs probed, status codes, and any model IDs
// surfaced — the wizard renders these to the operator so they can audit
// the auto-detection result.
type AzureProbeResult struct {
	Transport AzureTransport
	Models    []string
	Reason    string
	Evidence  []string
}

// ProbeAzureFoundry classifies an Azure Foundry endpoint by issuing two
// fixed probes:
//
//  1. GET <base>/models. On 200 + OpenAI-shaped JSON ({"data":[...]}}, the
//     transport is openai_chat_completions and the model IDs are returned
//     in Models. A 200 with an empty data list still classifies as OpenAI
//     ("shape OK, empty list").
//  2. POST <base>/v1/messages with a zero-token Anthropic Messages
//     payload. Any 4xx whose body mentions "messages" or "model"
//     classifies as anthropic_messages.
//
// When neither probe matches, Transport is unknown and Reason is
// "manual_required". The helper never writes to disk, never mutates
// configuration, and never retries beyond these two probes. The only
// non-nil error returned is the caller's context error when ctx is
// cancelled or deadlined mid-probe.
func ProbeAzureFoundry(ctx context.Context, client *http.Client, base, apiKey string) (AzureProbeResult, error) {
	if client == nil {
		client = http.DefaultClient
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")

	res := AzureProbeResult{Transport: AzureTransportUnknown}

	transport, models, modelsReason, modelsEvidence, err := probeAzureOpenAIModels(ctx, client, base, apiKey)
	if err != nil {
		return AzureProbeResult{}, err
	}
	res.Evidence = append(res.Evidence, modelsEvidence...)
	if transport == AzureTransportOpenAI {
		res.Transport = AzureTransportOpenAI
		res.Models = models
		res.Reason = modelsReason
		return res, nil
	}

	anthropic, anthReason, anthEvidence, err := probeAzureAnthropicMessages(ctx, client, base, apiKey)
	if err != nil {
		return AzureProbeResult{}, err
	}
	res.Evidence = append(res.Evidence, anthEvidence...)
	if anthropic {
		res.Transport = AzureTransportAnthropic
		res.Reason = anthReason
		return res, nil
	}

	res.Reason = "manual_required"
	return res, nil
}

func probeAzureOpenAIModels(ctx context.Context, client *http.Client, base, apiKey string) (AzureTransport, []string, string, []string, error) {
	url := base + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return AzureTransportUnknown, nil, "", []string{fmt.Sprintf("models_probe_request_error: %s", err.Error())}, nil
	}
	if apiKey != "" {
		req.Header.Set("api-key", apiKey)
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "gormes-agent/azure-foundry-probe")

	resp, err := client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return AzureTransportUnknown, nil, "", nil, ctxErr
		}
		return AzureTransportUnknown, nil, "", []string{fmt.Sprintf("models_probe_transport_error: %s", err.Error())}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	evidence := []string{fmt.Sprintf("GET %s -> %d", url, resp.StatusCode)}

	if resp.StatusCode != http.StatusOK {
		return AzureTransportUnknown, nil, "", evidence, nil
	}

	if !bytes.Contains(body, []byte(`"data"`)) {
		evidence = append(evidence, "models_probe_shape_mismatch: missing data field")
		return AzureTransportUnknown, nil, "", evidence, nil
	}

	var parsed struct {
		Object string `json:"object"`
		Data   []struct {
			ID    string `json:"id"`
			Model string `json:"model"`
			Name  string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		evidence = append(evidence, "models_probe_shape_mismatch: invalid JSON")
		return AzureTransportUnknown, nil, "", evidence, nil
	}

	ids := make([]string, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		switch {
		case item.ID != "":
			ids = append(ids, item.ID)
		case item.Model != "":
			ids = append(ids, item.Model)
		case item.Name != "":
			ids = append(ids, item.Name)
		}
	}

	if len(ids) == 0 {
		reason := "shape OK, empty list — OpenAI-style endpoint with no listed deployments"
		evidence = append(evidence, "models_probe_empty_list")
		return AzureTransportOpenAI, []string{}, reason, evidence, nil
	}

	reason := fmt.Sprintf("GET /models returned %d model(s) — OpenAI-style endpoint", len(ids))
	for _, id := range ids {
		evidence = append(evidence, "model_id="+id)
	}
	return AzureTransportOpenAI, ids, reason, evidence, nil
}

func probeAzureAnthropicMessages(ctx context.Context, client *http.Client, base, apiKey string) (bool, string, []string, error) {
	url := strings.TrimSuffix(base, "/v1") + "/v1/messages"
	payload := []byte(`{"model":"probe","max_tokens":1,"messages":[{"role":"user","content":"ping"}]}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return false, "", []string{fmt.Sprintf("anthropic_probe_request_error: %s", err.Error())}, nil
	}
	if apiKey != "" {
		req.Header.Set("api-key", apiKey)
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "gormes-agent/azure-foundry-probe")

	resp, err := client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, "", nil, ctxErr
		}
		return false, "", []string{fmt.Sprintf("anthropic_probe_transport_error: %s", err.Error())}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	evidence := []string{fmt.Sprintf("POST %s -> %d", url, resp.StatusCode)}
	lowered := strings.ToLower(string(body))

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		if strings.Contains(lowered, "messages") || strings.Contains(lowered, "model") || strings.Contains(lowered, "anthropic") {
			reason := fmt.Sprintf("POST /v1/messages returned %d with Anthropic-shaped error — Anthropic Messages endpoint", resp.StatusCode)
			evidence = append(evidence, "anthropic_probe_shape_match")
			return true, reason, evidence, nil
		}
		evidence = append(evidence, "anthropic_probe_shape_mismatch")
		return false, "", evidence, nil
	}

	evidence = append(evidence, "anthropic_probe_unexpected_status")
	return false, "", evidence, nil
}

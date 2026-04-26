package hermes

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// DetectAzureFoundry classifies an Azure Foundry endpoint into a transport
// read model from deterministic inputs only: the base URL, an injected
// HTTP client, and an API key string. It never reads or writes
// AZURE_FOUNDRY_BASE_URL, AZURE_FOUNDRY_API_KEY, deployment config, or
// model context metadata.
//
// Decision order, mirroring hermes_cli/azure_detect.py:detect:
//
//  1. Path sniff. If ClassifyAzurePath reports AzureTransportAnthropic,
//     return Transport=anthropic_messages immediately without HTTP. The
//     evidence records the parsed scheme/host/path so an operator can
//     audit the heuristic.
//  2. HTTP probes. Otherwise delegate to ProbeAzureFoundry, which probes
//     <base>/models for an OpenAI-shaped catalog and falls through to
//     <base>/v1/messages for an Anthropic Messages-shaped error.
//  3. Manual fallback. When neither classification fires, return
//     Transport=unknown with Reason="manual_required". The wizard treats
//     this as a non-fatal signal that manual api_mode entry is required.
//
// Models, when present, are advisory only - the helper does not persist
// them. The only non-nil error returned is the caller's context error
// when ctx is cancelled or deadlined mid-probe.
func DetectAzureFoundry(ctx context.Context, client *http.Client, base, apiKey string) (AzureProbeResult, error) {
	if transport := ClassifyAzurePath(base); transport == AzureTransportAnthropic {
		return AzureProbeResult{
			Transport: AzureTransportAnthropic,
			Reason:    "URL path sniff matched /anthropic - Anthropic Messages API",
			Evidence:  []string{azurePathSniffEvidence(base)},
		}, nil
	}

	res, err := ProbeAzureFoundry(ctx, client, base, apiKey)
	if err != nil {
		return AzureProbeResult{}, err
	}
	return res, nil
}

func azurePathSniffEvidence(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil {
		return "azure_path_sniff: anthropic"
	}
	scheme := parsed.Scheme
	host := parsed.Host
	path := strings.TrimRight(parsed.Path, "/")
	return fmt.Sprintf("azure_path_sniff: scheme=%s host=%s path=%s -> anthropic_messages", scheme, host, path)
}

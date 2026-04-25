package hermes

import "strings"

type ModelRouteReason string

const (
	ModelRouteReasonPrimary             ModelRouteReason = "primary"
	ModelRouteReasonTurnOverride        ModelRouteReason = "turn_override"
	ModelRouteReasonConfigOverride      ModelRouteReason = "config_override"
	ModelRouteReasonAutomaticSimpleTurn ModelRouteReason = "automatic_simple_turn"
	ModelRouteReasonFallbackSelected    ModelRouteReason = "fallback_selected"
	ModelRouteReasonFallbackDisabled    ModelRouteReason = "fallback_disabled"
	ModelRouteReasonFallbackUnavailable ModelRouteReason = "fallback_unavailable"
)

type ModelRoutingStatusCode string

const (
	ModelRoutingStatusProviderUnavailable   ModelRoutingStatusCode = "provider_unavailable"
	ModelRoutingStatusMetadataGap           ModelRoutingStatusCode = "metadata_gap"
	ModelRoutingStatusInvalidOverride       ModelRoutingStatusCode = "invalid_override"
	ModelRoutingStatusFallbackDisabled      ModelRoutingStatusCode = "fallback_disabled"
	ModelRoutingStatusFallbackUnavailable   ModelRoutingStatusCode = "fallback_unavailable"
	ModelRoutingStatusAutomaticRouteSkipped ModelRoutingStatusCode = "automatic_route_skipped"
)

type ModelRoute struct {
	Provider string
	Model    string
}

type ProviderAvailability struct {
	Provider  string
	Available bool
	Reason    string
}

type AutomaticModelRoutingPolicy struct {
	Enabled        bool
	SimpleRoute    ModelRoute
	MaxSimpleChars int
	MaxSimpleWords int
}

type FallbackModelPolicy struct {
	Enabled bool
	Routes  []ModelRoute
}

type ModelRoutingRequest struct {
	UserMessage       string
	Primary           ModelRoute
	ConfigOverride    ModelRoute
	TurnOverride      ModelRoute
	Automatic         AutomaticModelRoutingPolicy
	FallbackRequested bool
	Fallback          FallbackModelPolicy
	Providers         []ProviderAvailability
}

type ModelRoutingStatus struct {
	Code    ModelRoutingStatusCode
	Route   ModelRoute
	Message string
}

type ModelRoutingDecision struct {
	Route    ModelRoute
	Reason   ModelRouteReason
	Metadata ModelMetadataResult
	Context  ModelContextResolution
	Status   []ModelRoutingStatus
}

type ModelRouterConfig struct {
	Registry        ModelRegistry
	ContextResolver ModelContextResolver
}

type ModelRouter struct {
	registry        ModelRegistry
	contextResolver ModelContextResolver
}

func NewModelRouter(config ModelRouterConfig) ModelRouter {
	if config.Registry.entries == nil {
		config.Registry = DefaultModelRegistry()
	}
	if config.ContextResolver.providerCaps == nil {
		config.ContextResolver = DefaultModelContextResolver()
	}
	return ModelRouter{
		registry:        config.Registry,
		contextResolver: config.ContextResolver,
	}
}

func (r ModelRouter) Select(request ModelRoutingRequest) ModelRoutingDecision {
	availability := newProviderAvailabilityIndex(request.Providers)
	var status []ModelRoutingStatus

	if !request.TurnOverride.empty() {
		evaluated := r.evaluateRoute(request.TurnOverride, availability, &status)
		if evaluated.valid() {
			return decisionFromRoute(evaluated, ModelRouteReasonTurnOverride, status)
		}
		status = append(status, ModelRoutingStatus{
			Code:    ModelRoutingStatusInvalidOverride,
			Route:   evaluated.route,
			Message: invalidOverrideMessage("turn override", evaluated),
		})
	}

	if !request.ConfigOverride.empty() {
		evaluated := r.evaluateRoute(request.ConfigOverride, availability, &status)
		if evaluated.valid() {
			return decisionFromRoute(evaluated, ModelRouteReasonConfigOverride, status)
		}
		status = append(status, ModelRoutingStatus{
			Code:    ModelRoutingStatusInvalidOverride,
			Route:   evaluated.route,
			Message: invalidOverrideMessage("config override", evaluated),
		})
	}

	primary := r.evaluateRoute(request.Primary, availability, &status)
	if request.FallbackRequested {
		if !request.Fallback.Enabled {
			status = append(status, ModelRoutingStatus{
				Code:    ModelRoutingStatusFallbackDisabled,
				Route:   primary.route,
				Message: "fallback policy disabled",
			})
			return decisionFromRoute(primary, ModelRouteReasonFallbackDisabled, status)
		}
		for _, route := range request.Fallback.Routes {
			evaluated := r.evaluateRoute(route, availability, &status)
			if evaluated.valid() {
				return decisionFromRoute(evaluated, ModelRouteReasonFallbackSelected, status)
			}
		}
		status = append(status, ModelRoutingStatus{
			Code:    ModelRoutingStatusFallbackUnavailable,
			Route:   primary.route,
			Message: "no fallback route is available",
		})
		return decisionFromRoute(primary, ModelRouteReasonFallbackUnavailable, status)
	}

	if request.Automatic.Enabled {
		if simpleTurnSignal(request.UserMessage, request.Automatic) {
			evaluated := r.evaluateRoute(request.Automatic.SimpleRoute, availability, &status)
			if evaluated.valid() {
				return decisionFromRoute(evaluated, ModelRouteReasonAutomaticSimpleTurn, status)
			}
			status = append(status, ModelRoutingStatus{
				Code:    ModelRoutingStatusAutomaticRouteSkipped,
				Route:   evaluated.route,
				Message: "automatic route unavailable",
			})
		} else {
			status = append(status, ModelRoutingStatus{
				Code:    ModelRoutingStatusAutomaticRouteSkipped,
				Route:   normalizeModelRoute(request.Automatic.SimpleRoute),
				Message: "complex task signal",
			})
		}
	}

	return decisionFromRoute(primary, ModelRouteReasonPrimary, status)
}

type evaluatedModelRoute struct {
	route             ModelRoute
	metadata          ModelMetadataResult
	context           ModelContextResolution
	providerAvailable bool
	metadataAvailable bool
	contextAvailable  bool
}

func (r ModelRouter) evaluateRoute(route ModelRoute, availability providerAvailabilityIndex, status *[]ModelRoutingStatus) evaluatedModelRoute {
	route = normalizeModelRoute(route)
	evaluated := evaluatedModelRoute{
		route:             route,
		providerAvailable: true,
	}
	if route.empty() {
		evaluated.metadata = r.registry.Lookup(ModelRegistryQuery{})
		evaluated.context = r.contextResolver.Resolve(ModelContextQuery{})
		return evaluated
	}

	if ok, reason := availability.available(route.Provider); !ok {
		evaluated.providerAvailable = false
		message := route.Provider + " unavailable"
		if reason != "" {
			message += ": " + reason
		}
		*status = append(*status, ModelRoutingStatus{
			Code:    ModelRoutingStatusProviderUnavailable,
			Route:   route,
			Message: message,
		})
	}

	evaluated.metadata = r.registry.Lookup(ModelRegistryQuery{
		Provider: route.Provider,
		Model:    route.Model,
	})
	evaluated.metadataAvailable = evaluated.metadata.Found
	if !evaluated.metadataAvailable {
		*status = append(*status, ModelRoutingStatus{
			Code:    ModelRoutingStatusMetadataGap,
			Route:   route,
			Message: "missing metadata for " + route.Provider + "/" + route.Model,
		})
	}

	evaluated.context = r.contextResolver.Resolve(ModelContextQuery{
		Provider: route.Provider,
		Model:    route.Model,
		ModelInfo: ModelContextMetadata{
			ContextWindow: evaluated.metadata.RawContextWindow,
		},
	})
	evaluated.contextAvailable = evaluated.context.Known()
	if evaluated.metadataAvailable && !evaluated.contextAvailable {
		*status = append(*status, ModelRoutingStatus{
			Code:    ModelRoutingStatusMetadataGap,
			Route:   route,
			Message: "missing context limit for " + route.Provider + "/" + route.Model,
		})
	}
	if evaluated.metadataAvailable && !evaluated.metadata.Pricing.Known() {
		*status = append(*status, ModelRoutingStatus{
			Code:    ModelRoutingStatusMetadataGap,
			Route:   route,
			Message: "missing pricing for " + route.Provider + "/" + route.Model,
		})
	}
	if evaluated.metadataAvailable && !evaluated.metadata.Capabilities.Known() {
		*status = append(*status, ModelRoutingStatus{
			Code:    ModelRoutingStatusMetadataGap,
			Route:   route,
			Message: "missing capabilities for " + route.Provider + "/" + route.Model,
		})
	}
	return evaluated
}

func (r evaluatedModelRoute) valid() bool {
	return !r.route.empty() && r.providerAvailable && r.metadataAvailable && r.contextAvailable
}

func decisionFromRoute(route evaluatedModelRoute, reason ModelRouteReason, status []ModelRoutingStatus) ModelRoutingDecision {
	return ModelRoutingDecision{
		Route:    route.route,
		Reason:   reason,
		Metadata: route.metadata,
		Context:  route.context,
		Status:   append([]ModelRoutingStatus(nil), status...),
	}
}

func invalidOverrideMessage(label string, route evaluatedModelRoute) string {
	if route.route.empty() {
		return label + " is incomplete"
	}
	if !route.providerAvailable {
		return label + " provider is unavailable"
	}
	if !route.metadataAvailable {
		return label + " model metadata is unavailable"
	}
	if !route.contextAvailable {
		return label + " context limit is unavailable"
	}
	return label + " is unavailable"
}

type providerAvailabilityIndex map[string]ProviderAvailability

func newProviderAvailabilityIndex(providers []ProviderAvailability) providerAvailabilityIndex {
	index := make(providerAvailabilityIndex, len(providers))
	for _, provider := range providers {
		normalized := normalizeModelContextProvider(provider.Provider)
		if normalized == "" {
			continue
		}
		provider.Provider = normalized
		index[normalized] = provider
	}
	return index
}

func (i providerAvailabilityIndex) available(provider string) (bool, string) {
	if len(i) == 0 {
		return true, ""
	}
	status, ok := i[normalizeModelContextProvider(provider)]
	if !ok {
		return false, "provider availability unknown"
	}
	return status.Available, strings.TrimSpace(status.Reason)
}

func normalizeModelRoute(route ModelRoute) ModelRoute {
	return ModelRoute{
		Provider: normalizeModelContextProvider(route.Provider),
		Model:    normalizeModelContextText(route.Model),
	}
}

func (r ModelRoute) empty() bool {
	return strings.TrimSpace(r.Provider) == "" && strings.TrimSpace(r.Model) == ""
}

func simpleTurnSignal(message string, policy AutomaticModelRoutingPolicy) bool {
	text := strings.TrimSpace(message)
	if text == "" {
		return false
	}
	maxChars := policy.MaxSimpleChars
	if maxChars <= 0 {
		maxChars = 160
	}
	maxWords := policy.MaxSimpleWords
	if maxWords <= 0 {
		maxWords = 28
	}
	if len(text) > maxChars {
		return false
	}
	words := strings.Fields(text)
	if len(words) > maxWords {
		return false
	}
	if strings.Count(text, "\n") > 1 {
		return false
	}
	lowered := strings.ToLower(text)
	if strings.Contains(lowered, "```") || strings.Contains(lowered, "`") {
		return false
	}
	if strings.Contains(lowered, "http://") || strings.Contains(lowered, "https://") || strings.Contains(lowered, "www.") {
		return false
	}
	for _, word := range words {
		normalized := strings.ToLower(strings.Trim(word, ".,:;!?()[]{}\"'`"))
		if complexRoutingKeywords[normalized] {
			return false
		}
	}
	return true
}

var complexRoutingKeywords = map[string]bool{
	"debug":          true,
	"debugging":      true,
	"implement":      true,
	"implementation": true,
	"refactor":       true,
	"patch":          true,
	"traceback":      true,
	"stacktrace":     true,
	"exception":      true,
	"error":          true,
	"analyze":        true,
	"analysis":       true,
	"investigate":    true,
	"architecture":   true,
	"design":         true,
	"compare":        true,
	"benchmark":      true,
	"optimize":       true,
	"optimise":       true,
	"review":         true,
	"terminal":       true,
	"shell":          true,
	"tool":           true,
	"tools":          true,
	"pytest":         true,
	"test":           true,
	"tests":          true,
	"plan":           true,
	"planning":       true,
	"delegate":       true,
	"subagent":       true,
	"cron":           true,
	"docker":         true,
	"kubernetes":     true,
}

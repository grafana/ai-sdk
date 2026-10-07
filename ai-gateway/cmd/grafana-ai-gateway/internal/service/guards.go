package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/outbound"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/prometheus/client_golang/prometheus"
)

// GuardRuntime is the independent operator-owned Sigil client. Admission spans
// both phases and model work, rather than just an individual HTTP request.
// It never uses caller headers or the generation-recording client.
type GuardRuntime struct {
	settings    config.GuardSettings
	endpoint    string
	secret      string
	client      *http.Client
	slots       chan struct{}
	mu          sync.Mutex
	closed      bool
	lifecycle   context.Context
	cancel      context.CancelFunc
	registerer  prometheus.Registerer
	collectors  []prometheus.Collector
	evaluations *prometheus.CounterVec
	duration    *prometheus.HistogramVec
}

// NewGuardRuntime constructs operator policy dependencies before listener binding.
// Disabled guards return nil without resolving credentials.
func NewGuardRuntime(settings config.GuardSettings, mode config.DeploymentMode, lookupEnv config.LookupEnv, telemetry *Telemetry) (*GuardRuntime, error) {
	if !settings.Enabled {
		return nil, nil
	}
	if err := validateGuardRuntimeSettings(settings); err != nil {
		return nil, err
	}
	endpoint, err := outbound.ValidateEndpoint(settings.Endpoint, mode)
	if err != nil {
		return nil, fmt.Errorf("gateway guards: invalid endpoint")
	}
	secret, err := settings.ResolveAuthSecret(lookupEnv)
	if err != nil {
		return nil, err
	}
	if telemetry == nil {
		return nil, fmt.Errorf("gateway guards: telemetry is required")
	}
	client, err := outbound.NewGuardClient(settings.Timeout, settings.BodyBytes)
	if err != nil {
		return nil, err
	}
	lifecycle, cancel := context.WithCancel(context.Background())
	runtime := &GuardRuntime{settings: settings, endpoint: endpoint.JoinPath("api/v1/hooks:evaluate").String(), secret: secret, client: client, slots: make(chan struct{}, settings.MaxConcurrent), lifecycle: lifecycle, cancel: cancel, registerer: telemetry.Registerer(),
		evaluations: prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: "grafana_ai_gateway", Name: "guard_evaluations_total", Help: "Guard decisions by fixed phase and outcome."}, []string{"phase", "outcome"}),
		duration:    prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: "grafana_ai_gateway", Name: "guard_evaluation_duration_seconds", Help: "Guard evaluation duration by fixed phase and outcome."}, []string{"phase", "outcome"})}
	for _, collector := range []prometheus.Collector{runtime.evaluations, runtime.duration} {
		if err := runtime.registerer.Register(collector); err != nil {
			runtime.Close()
			return nil, fmt.Errorf("gateway guards: registering metrics failed")
		}
		runtime.collectors = append(runtime.collectors, collector)
	}
	return runtime, nil
}

func validateGuardRuntimeSettings(s config.GuardSettings) error {
	validValue := func(value string) bool {
		if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
			return false
		}
		for _, r := range value {
			if unicode.IsControl(r) {
				return false
			}
		}
		return true
	}
	if !validValue(s.Endpoint) || !validValue(s.TenantID) {
		return fmt.Errorf("gateway guards: invalid endpoint or tenant")
	}
	switch s.AuthMode {
	case config.GuardAuthModeBasic:
		if !validValue(s.AuthUsername) || strings.Contains(s.AuthUsername, ":") {
			return fmt.Errorf("gateway guards: invalid basic authentication")
		}
	case config.GuardAuthModeBearer:
		if s.AuthUsername != "" {
			return fmt.Errorf("gateway guards: invalid bearer authentication")
		}
	default:
		return fmt.Errorf("gateway guards: invalid authentication mode")
	}
	if s.Timeout < time.Millisecond || s.Timeout > 119999*time.Millisecond || s.BodyBytes <= 0 || s.BodyBytes > 4<<20 || s.RetainedBytes <= 0 || s.RetainedBytes > 64<<20 || s.MaxConcurrent <= 0 || s.MaxConcurrent > 128 {
		return fmt.Errorf("gateway guards: invalid resource limits")
	}
	return nil
}

// Close is idempotent, stops outstanding evaluations, and releases transport and
// metric-registration resources, including partially constructed runtimes.
func (runtime *GuardRuntime) Close() {
	if runtime == nil {
		return
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return
	}
	runtime.closed = true
	runtime.cancel()
	runtime.client.CloseIdleConnections()
	for _, collector := range runtime.collectors {
		runtime.registerer.Unregister(collector)
	}
}

// Acquire performs nonblocking process-runtime admission. The caller owns the
// idempotent release and holds it until the entire guarded operation completes.
func (runtime *GuardRuntime) Acquire(ctx context.Context) (func(), error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := ctx.Err(); err != nil {
		runtime.observe(guardPreflight, "canceled", time.Now())
		return nil, err
	}
	if runtime.closed {
		return nil, providerv4.ErrGuardFailed
	}
	select {
	case runtime.slots <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-runtime.slots }) }, nil
	default:
		runtime.observe(guardPreflight, "resource_failure", time.Now())
		return nil, providerv4.ErrGuardFailed
	}
}

// Preflight uses resolved public identity rather than a fallback candidate identity.
func (runtime *GuardRuntime) Preflight(ctx context.Context, canonicalID string, options provider.CallOptions) (provider.CallOptions, error) {
	return runtime.evaluate(ctx, guardPreflight, canonicalID, options, nil)
}

// Postflight checks complete output with the effective request; transforms are unsupported.
func (runtime *GuardRuntime) Postflight(ctx context.Context, canonicalID string, options provider.CallOptions, output []provider.ContentPart) error {
	// Preserve an empty-but-complete output as postflight, never a preflight projection.
	if output == nil {
		output = []provider.ContentPart{}
	}
	_, err := runtime.evaluate(ctx, guardPostflight, canonicalID, options, output)
	return err
}

// A fixed agent identity is deliberate: it enables operator rule selection for
// Gateway traffic without borrowing caller identity or conversation metadata.
type guardHookRequest struct {
	Phase   guardPhase       `json:"phase"`
	Context guardHookContext `json:"context"`
	Input   guardInput       `json:"input"`
}
type guardHookContext struct {
	AgentName string         `json:"agent_name"`
	Model     guardHookModel `json:"model"`
}
type guardHookModel struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

func (runtime *GuardRuntime) evaluate(ctx context.Context, phase guardPhase, canonicalID string, options provider.CallOptions, output []provider.ContentPart) (effective provider.CallOptions, err error) {
	timeout := runtime.settings.Timeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline) / 2; remaining < timeout {
			timeout = remaining
		}
	}
	if timeout < time.Millisecond {
		timeout = 0
	}
	phaseContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stop := context.AfterFunc(runtime.lifecycle, cancel)
	defer stop()
	return runtime.evaluatePhase(ctx, phaseContext, phase, canonicalID, options, output)
}

func (runtime *GuardRuntime) evaluatePhase(ctx, phaseContext context.Context, phase guardPhase, canonicalID string, options provider.CallOptions, output []provider.ContentPart) (effective provider.CallOptions, err error) {
	effective = options
	started := time.Now()
	outcome := "service_failure"
	transformPresent := false
	defer func() {
		if err == nil && outcome == "allow" && phaseContext.Err() != nil {
			effective = options
			switch {
			case transformPresent:
				err = providerv4.ErrGuardFailed
				outcome = "transform_failure"
			case runtime.settings.FailOpen:
				outcome = "fail_open"
			default:
				err = providerv4.ErrGuardFailed
				outcome = "service_failure"
			}
		}
		// Parent cancellation always wins, including a late allow, deny, or timeout.
		if parentErr := ctx.Err(); parentErr != nil {
			err = parentErr
			effective = options
			outcome = "canceled"
		} else if runtime.lifecycle.Err() != nil {
			err = providerv4.ErrGuardFailed
			effective = options
			outcome = "resource_failure"
		}
		runtime.observe(phase, outcome, started)
	}()
	fail := func(class string, cause error) (provider.CallOptions, error) { outcome = class; return options, cause }
	serviceFailure := func() (provider.CallOptions, error) {
		if ctx.Err() != nil {
			return fail("canceled", ctx.Err())
		}
		if runtime.lifecycle.Err() != nil {
			return fail("resource_failure", providerv4.ErrGuardFailed)
		}
		if runtime.settings.FailOpen {
			outcome = "fail_open"
			return options, nil
		}
		return fail("service_failure", providerv4.ErrGuardFailed)
	}
	if ctx.Err() != nil {
		return fail("canceled", ctx.Err())
	}
	if runtime.lifecycle.Err() != nil {
		return fail("resource_failure", providerv4.ErrGuardFailed)
	}
	if _, ok := phaseContext.Deadline(); !ok || phaseContext.Err() != nil {
		return fail("resource_failure", providerv4.ErrGuardFailed)
	}
	// Projection and withheld output coexist, so each gets half the guard budget.
	budget := runtime.settings.RetainedBytes / 2
	if !guardReserveGraphContext(phaseContext, reflect.ValueOf(options), &budget, 0) || !guardReserveGraphContext(phaseContext, reflect.ValueOf(output), &budget, 0) || !guardReserveGraphContext(phaseContext, reflect.ValueOf(canonicalID), &budget, 0) {
		return fail("resource_failure", providerv4.ErrGuardFailed)
	}
	input, mapErr := makeGuardInputContext(phaseContext, options, output)
	if mapErr != nil {
		if phaseContext.Err() != nil {
			return fail("resource_failure", providerv4.ErrGuardFailed)
		}
		if errors.Is(mapErr, errGuardUnsupported) {
			if phase == guardPostflight {
				return fail("unsupported", providerv4.ErrGuardFailed)
			}
			return fail("unsupported", providerv4.ErrGuardUnsupported)
		}
		return fail("resource_failure", providerv4.ErrGuardFailed)
	}
	requestBody := &guardBoundedBuffer{limit: runtime.settings.BodyBytes, ctx: phaseContext}
	encodeErr := json.NewEncoder(requestBody).Encode(guardHookRequest{Phase: phase, Context: guardHookContext{AgentName: "grafana-ai-gateway", Model: guardHookModel{Provider: "grafana", Name: canonicalID}}, Input: input})
	if encodeErr != nil {
		return fail("resource_failure", providerv4.ErrGuardFailed)
	}
	request, requestErr := http.NewRequestWithContext(phaseContext, http.MethodPost, runtime.endpoint, bytes.NewReader(requestBody.Bytes()))
	if requestErr != nil {
		return serviceFailure()
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Scope-OrgID", runtime.settings.TenantID)
	switch runtime.settings.AuthMode {
	case config.GuardAuthModeBasic:
		request.SetBasicAuth(runtime.settings.AuthUsername, runtime.secret)
	case config.GuardAuthModeBearer:
		request.Header.Set("Authorization", "Bearer "+runtime.secret)
	}
	deadline, _ := phaseContext.Deadline()
	millis := time.Until(deadline).Milliseconds()
	if millis < 1 {
		return serviceFailure()
	}
	if millis > 119999 {
		millis = 119999
	}
	request.Header.Set("X-Agento11y-Hook-Timeout-Ms", strconv.FormatInt(millis, 10))
	response, requestErr := runtime.client.Do(request)
	if requestErr != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return serviceFailure()
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return serviceFailure()
	}

	responseLimit := min(runtime.settings.BodyBytes, budget/guardAllocationFactor)
	if responseLimit < 1 {
		return fail("resource_failure", providerv4.ErrGuardFailed)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if int64(len(body)) > responseLimit || errors.Is(readErr, outbound.ErrResponseTooLarge) {
		return fail("resource_failure", providerv4.ErrGuardFailed)
	}
	if readErr != nil {
		return serviceFailure()
	}
	verdict, decodeErr := decodeGuardRuntimeResponseContext(phaseContext, body)
	if phaseContext.Err() != nil {
		authorityContext, cancelAuthority := context.WithCancel(ctx)
		stopAuthority := context.AfterFunc(runtime.lifecycle, cancelAuthority)
		verdict, decodeErr = decodeGuardRuntimeResponseContext(authorityContext, body)
		stopAuthority()
		cancelAuthority()
	}
	if decodeErr != nil {
		if errors.Is(decodeErr, errGuardTransform) {
			return fail("transform_failure", providerv4.ErrGuardFailed)
		}
		return serviceFailure()
	}
	if verdict.action == guardDeny {
		return fail("deny", providerv4.ErrGuardDenied)
	}
	transformPresent = verdict.transformPresent
	if verdict.transformPresent {
		if phase == guardPostflight || phaseContext.Err() != nil {
			return fail("transform_failure", providerv4.ErrGuardFailed)
		}
		transformed, transformErr := applyGuardTransformContext(phaseContext, options, input, verdict.transform)
		if transformErr != nil {
			return fail("transform_failure", providerv4.ErrGuardFailed)
		}
		outcome = "allow"
		return transformed, nil
	}
	outcome = "allow"
	return options, nil
}

// Optional diagnostics cannot override a denial or make transforms fail open.
func decodeGuardRuntimeResponseContext(ctx context.Context, body []byte) (guardVerdict, error) {
	if ctx.Err() != nil {
		return guardVerdict{}, ctx.Err()
	}
	decoder := json.NewDecoder(&guardContextReader{ctx: ctx, reader: bytes.NewReader(body)})
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return guardVerdict{}, errGuardResponse
	}
	seen := map[string]bool{}
	authoritative := map[string]json.RawMessage{}
	duplicateTransform := false
	for decoder.More() {
		if ctx.Err() != nil {
			return guardVerdict{}, ctx.Err()
		}
		token, err = decoder.Token()
		if err != nil {
			return guardVerdict{}, errGuardResponse
		}
		key, ok := token.(string)
		if !ok {
			return guardVerdict{}, errGuardResponse
		}
		if seen[key] {
			switch key {
			case "action":
				return guardVerdict{}, errGuardResponse
			case "transformed_input":
				duplicateTransform = true
			}
		}
		seen[key] = true
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return guardVerdict{}, errGuardResponse
		}
		if key == "action" || key == "transformed_input" {
			authoritative[key] = raw
		}
	}
	if _, err = decoder.Token(); err != nil {
		return guardVerdict{}, errGuardResponse
	}
	if _, err = decoder.Token(); err != io.EOF {
		return guardVerdict{}, errGuardResponse
	}
	if ctx.Err() != nil {
		return guardVerdict{}, ctx.Err()
	}
	var verdict guardVerdict
	if json.Unmarshal(authoritative["action"], &verdict.action) != nil || (verdict.action != guardAllow && verdict.action != guardDeny) {
		return guardVerdict{}, errGuardResponse
	}
	verdict.transform, verdict.transformPresent = authoritative["transformed_input"]
	if verdict.action == guardAllow && duplicateTransform {
		return guardVerdict{}, errGuardTransform
	}
	return verdict, nil
}

const guardAllocationFactor int64 = 16

// The multiplier reserves serialization and projection scratch, not provider heap.
func guardReserveGraphContext(ctx context.Context, value reflect.Value, budget *int64, depth int) bool {
	if ctx.Err() != nil {
		return false
	}
	if !value.IsValid() {
		return true
	}
	if depth > 64 {
		return false
	}
	charge := func(size int64) bool {
		if size < 0 || size > *budget/guardAllocationFactor {
			return false
		}
		*budget -= size * guardAllocationFactor
		return true
	}
	if !charge(int64(value.Type().Size()) + 16) {
		return false
	}
	switch value.Kind() {
	case reflect.String:
		return charge(int64(value.Len()))
	case reflect.Interface, reflect.Pointer:
		if !value.IsNil() {
			return guardReserveGraphContext(ctx, value.Elem(), budget, depth+1)
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if !guardReserveGraphContext(ctx, value.Field(i), budget, depth+1) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return charge(int64(value.Len()))
		}
		for i := 0; i < value.Len(); i++ {
			if !guardReserveGraphContext(ctx, value.Index(i), budget, depth+1) {
				return false
			}
		}
	case reflect.Map:
		iterator := value.MapRange()
		for iterator.Next() {
			if !charge(128) || !guardReserveGraphContext(ctx, iterator.Key(), budget, depth+1) || !guardReserveGraphContext(ctx, iterator.Value(), budget, depth+1) {
				return false
			}
		}
	}
	return true
}

type guardBoundedBuffer struct {
	bytes.Buffer
	limit int64
	ctx   context.Context
}

func (buffer *guardBoundedBuffer) Write(data []byte) (int, error) {
	if buffer.ctx != nil && buffer.ctx.Err() != nil {
		return 0, buffer.ctx.Err()
	}
	if int64(len(data)) > buffer.limit-int64(buffer.Len()) {
		return 0, providerv4.ErrGuardFailed
	}
	return buffer.Buffer.Write(data)
}
func (runtime *GuardRuntime) observe(phase guardPhase, outcome string, started time.Time) {
	switch phase {
	case guardPreflight, guardPostflight:
	default:
		return
	}
	switch outcome {
	case "allow", "deny", "fail_open", "service_failure", "transform_failure", "unsupported", "canceled", "resource_failure":
	default:
		outcome = "service_failure"
	}
	runtime.evaluations.WithLabelValues(string(phase), outcome).Inc()
	runtime.duration.WithLabelValues(string(phase), outcome).Observe(time.Since(started).Seconds())
}

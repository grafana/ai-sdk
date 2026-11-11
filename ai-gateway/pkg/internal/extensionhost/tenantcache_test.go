package extensionhost

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

const tenantSentinel = "sentinel-tenant-value"

type namespaceKey struct{}

func inNamespace(ctx context.Context, namespace string) context.Context {
	return context.WithValue(ctx, namespaceKey{}, namespace)
}

// fakeReader serves stored resources per namespace. When gated, each read
// waits for a receive on the gate or for its context to end.
type fakeReader struct {
	mu       sync.Mutex
	stored   map[string][]TenantResource
	err      error
	gate     chan struct{}
	reads    []string
	contexts []context.Context
}

func newFakeReader() *fakeReader { return &fakeReader{stored: map[string][]TenantResource{}} }

func (r *fakeReader) Read(ctx context.Context, namespace string) ([]TenantResource, error) {
	r.mu.Lock()
	r.reads = append(r.reads, namespace)
	r.contexts = append(r.contexts, ctx)
	gate := r.gate
	r.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ErrReadTimeout
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	return slices.Clone(r.stored[namespace]), nil
}

func (r *fakeReader) set(namespace string, resources ...TenantResource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stored[namespace] = resources
}

func (r *fakeReader) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

func (r *fakeReader) setGate(gate chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gate = gate
}

func (r *fakeReader) readCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reads)
}

func stored(name string, enabled bool, config string) TenantResource {
	return TenantResource{Name: name, Type: name, Enabled: &enabled, Config: json.RawMessage(config)}
}

func mustSchema(t *testing.T, raw string) schema.Schema {
	t.Helper()
	compiled, err := schema.SchemaFromJSON(json.RawMessage(raw))
	require.NoError(t, err)
	return compiled
}

// configurable returns a default-off, skip tenant extension whose transform
// journals its name and configuration.
func configurable(t *testing.T, name string, log *journal, change ...func(*extension.Definition)) extension.Definition {
	t.Helper()
	valueSchema := mustSchema(t, `{"type":"object","properties":{"v":{"type":"string"},"n":{"type":"integer"}},"additionalProperties":false}`)
	base := []func(*extension.Definition){tenant(0), skip, func(d *extension.Definition) {
		d.EnabledByDefault = false
		d.ConfigSchema = valueSchema
		d.New = func(config json.RawMessage, _ extension.Dependencies) (any, error) {
			return &recorder{name: name + string(config), journal: log}, nil
		}
	}}
	return define(name, nil, append(base, change...)...)
}

func defaultOn(definition *extension.Definition) { definition.EnabledByDefault = true }

func failClosed(definition *extension.Definition) { definition.FailurePolicy = extension.FailClosed }

// newTenantHarness returns a harness whose calls carry the namespace of
// their context, connected to reader.
func newTenantHarness(t *testing.T, reader TenantReader, cache TenantCacheConfig, definitions ...extension.Definition) *harness {
	t.Helper()
	h := newHarnessWith(t, func(config *Config) {
		config.TenantCache = cache
		config.RequestInfo = func(ctx context.Context) extension.Call {
			namespace, _ := ctx.Value(namespaceKey{}).(string)
			return extension.Call{Namespace: namespace, Caller: extension.CallerServiceAccount, CorrelationID: "correlation-1"}
		}
	}, definitions...)
	h.host.ConnectTenants(reader)
	return h
}

// generate makes one unary operator-catalog call in namespace.
func (h *harness) generate(t *testing.T, namespace string) error {
	t.Helper()
	_, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(inNamespace(t.Context(), namespace), provider.CallOptions{})
	return err
}

// transforms returns the journal's transform entries in order.
func transforms(log *journal) []string {
	var ran []string
	for _, entry := range log.list() {
		if name, ok := strings.CutSuffix(entry, ":transform"); ok {
			ran = append(ran, name)
		}
	}
	return ran
}

func metricValue(h *harness, name string, labels map[string]string) float64 {
	return counterValue(h.registry, name, labels)
}

func TestTenantCache_ConcurrentFirstCallsShareOneRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		gate := make(chan struct{})
		reader.setGate(gate)
		log := &journal{}
		reader.set("a", stored("t1", true, `{"v":"a"}`))
		h := newTenantHarness(t, reader, TenantCacheConfig{}, configurable(t, "t1", log))

		var calls sync.WaitGroup
		for range 10 {
			calls.Go(func() { assert.NoError(t, h.generate(t, "a")) })
		}
		synctest.Wait()
		assert.Equal(t, 1, reader.readCount())
		close(gate)
		calls.Wait()
		assert.Equal(t, 1, reader.readCount())
		assert.Len(t, transforms(log), 10)
		assert.Equal(t, []string{`t1{"v":"a"}`}, slices.Compact(transforms(log)))
	})
}

func TestTenantCache_ReadsAreDetachedFromTheCaller(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		gate := make(chan struct{})
		reader.setGate(gate)
		log := &journal{}
		reader.set("a", stored("t1", true, `{"v":"a"}`))
		h := newTenantHarness(t, reader, TenantCacheConfig{}, configurable(t, "t1", log))

		ctx, cancel := context.WithCancel(inNamespace(t.Context(), "a"))
		done := make(chan error, 1)
		go func() {
			_, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(ctx, provider.CallOptions{})
			done <- err
		}()
		synctest.Wait()
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
		assert.Zero(t, h.model.callCount())

		reader.mu.Lock()
		read := reader.contexts[0]
		reader.mu.Unlock()
		assert.NoError(t, read.Err(), "the read outlives the caller")
		assert.Nil(t, read.Value(namespaceKey{}), "no value of the caller's context, such as its identity, reaches the read")
		_, hasDeadline := read.Deadline()
		assert.True(t, hasDeadline, "the read has its own timeout")

		close(gate)
		synctest.Wait()
		require.NoError(t, h.generate(t, "a"))
		assert.Equal(t, 1, reader.readCount())
		assert.Equal(t, []string{`t1{"v":"a"}`}, transforms(log))
	})
}

func TestTenantCache_ReadTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.setGate(make(chan struct{}))
		log := &journal{}
		h := newTenantHarness(t, reader, TenantCacheConfig{}, configurable(t, "t1", log, defaultOn))

		start := time.Now()
		require.NoError(t, h.generate(t, "a"))
		assert.Equal(t, defaultTenantReadTimeout, time.Since(start))
		assert.Empty(t, transforms(log), "a failed read never applies the default")
		assert.Equal(t, 1.0, metricValue(h, "grafana_ai_gateway_tenant_config_reads_total", map[string]string{"result": "failed"}))
		assert.Contains(t, h.logs.String(), `"class":"timeout"`)
	})
}

func TestTenantCache_RefreshAfterTheTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		log := &journal{}
		reader.set("a", stored("t1", true, `{"v":"old"}`))
		h := newTenantHarness(t, reader, TenantCacheConfig{TTL: time.Minute}, configurable(t, "t1", log))

		require.NoError(t, h.generate(t, "a"))
		reader.set("a", stored("t1", true, `{"v":"new"}`))
		time.Sleep(time.Minute - time.Nanosecond)
		require.NoError(t, h.generate(t, "a"))
		synctest.Wait()
		assert.Equal(t, 1, reader.readCount(), "no read inside the TTL")

		time.Sleep(time.Nanosecond)
		require.NoError(t, h.generate(t, "a"))
		synctest.Wait()
		assert.Equal(t, 2, reader.readCount(), "one background refresh at the TTL")
		require.NoError(t, h.generate(t, "a"))
		assert.Equal(t, []string{`t1{"v":"old"}`, `t1{"v":"old"}`, `t1{"v":"old"}`, `t1{"v":"new"}`}, transforms(log))
		assert.Equal(t, 2, reader.readCount())
	})
}

func TestTenantCache_FailuresBackOffAndServeLastKnownGood(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		log := &journal{}
		reader.set("a", stored("off", false, `{}`))
		h := newTenantHarness(t, reader, TenantCacheConfig{TTL: time.Minute, FailureBackoff: 30 * time.Second, MaxStaleness: 10 * time.Minute},
			configurable(t, "off", log, defaultOn),
			configurable(t, "gate", log, failClosed),
		)
		require.NoError(t, h.generate(t, "a"))
		assert.Empty(t, transforms(log), "the stored resource disables the default-on extension")

		reader.fail(ErrReadRefused)
		time.Sleep(time.Minute)
		require.NoError(t, h.generate(t, "a"))
		synctest.Wait()
		assert.Equal(t, 2, reader.readCount())
		time.Sleep(30*time.Second - time.Nanosecond)
		require.NoError(t, h.generate(t, "a"))
		synctest.Wait()
		assert.Equal(t, 2, reader.readCount(), "no read before the backoff ends")
		time.Sleep(time.Nanosecond)
		require.NoError(t, h.generate(t, "a"))
		synctest.Wait()
		assert.Equal(t, 3, reader.readCount())

		time.Sleep(10*time.Minute - time.Minute - 30*time.Second - time.Nanosecond)
		require.NoError(t, h.generate(t, "a"))
		assert.Empty(t, transforms(log), "last-known-good keeps the extension disabled within the bound")
		assert.Zero(t, h.events("gate", string(phaseConfigure), eventFailed))

		time.Sleep(time.Nanosecond)
		err := h.generate(t, "a")
		require.ErrorIs(t, err, extension.ErrUnavailable, "the fail-closed extension refuses after the bound")
		assert.Empty(t, transforms(log), "the skip extension stays off instead of using its default")
		calls := h.model.callCount()
		assert.Contains(t, h.logs.String(), "gateway tenant configuration expired")
		assert.Contains(t, h.logs.String(), `"class":"refused"`)

		reader.fail(nil)
		synctest.Wait()
		time.Sleep(30 * time.Second)
		_ = h.generate(t, "a")
		synctest.Wait()
		require.NoError(t, h.generate(t, "a"))
		assert.Empty(t, transforms(log), "the restored configuration disables the extension again")
		assert.Equal(t, calls+1, h.model.callCount())
	})
}

func TestTenantCache_FirstReadFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.fail(ErrReadNotFound)
		log := &journal{}
		h := newTenantHarness(t, reader, TenantCacheConfig{},
			configurable(t, "default-on", log, defaultOn),
			configurable(t, "gate", log, failClosed),
			define("operator", &recorder{name: "operator", journal: log}),
		)
		err := h.generate(t, "a")
		require.ErrorIs(t, err, extension.ErrUnavailable)
		assert.Zero(t, h.model.callCount())
		assert.Empty(t, transforms(log))
		synctest.Wait()
		assert.Contains(t, log.list(), "operator:observe:error", "operator observers record the refusal")
		assert.Contains(t, h.logs.String(), `"class":"not-found"`)
	})
}

func TestTenantCache_Namespaces(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "unary", true: "stream"}[stream], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				reader := newFakeReader()
				log := &journal{}
				reader.set("a", stored("t1", true, `{"v":"a"}`))
				reader.set("b", stored("t1", true, `{"v":"b"}`))
				reader.set("d", stored("on", false, `{}`))
				h := newTenantHarness(t, reader, TenantCacheConfig{}, configurable(t, "t1", log), configurable(t, "on", log, defaultOn))
				model := h.compose(t, extension.SourceOperatorCatalog)
				for _, namespace := range []string{"a", "b", "c", "d"} {
					ctx := inNamespace(t.Context(), namespace)
					if stream {
						result, err := model.DoStream(ctx, provider.CallOptions{})
						require.NoError(t, err)
						collect(t, result.Stream)
					} else {
						_, err := model.DoGenerate(ctx, provider.CallOptions{})
						require.NoError(t, err)
					}
				}
				assert.Equal(t, []string{`on{}`, `t1{"v":"a"}`, `on{}`, `t1{"v":"b"}`, `on{}`}, transforms(log))
			})
		})
	}
}

func TestTenantCache_ReadsOnlyWhenATenantExtensionApplies(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		log := &journal{}
		h := newTenantHarness(t, reader, TenantCacheConfig{},
			configurable(t, "operator-keys", log, func(d *extension.Definition) { d.Keys = []extension.KeyOwnership{extension.KeysOperator} }),
			define("operator", &recorder{name: "operator", journal: log}),
		)
		_, err := h.compose(t, extension.SourceRequestBYOK).DoGenerate(inNamespace(t.Context(), "a"), provider.CallOptions{})
		require.NoError(t, err)
		assert.Zero(t, reader.readCount())
		require.NoError(t, h.generate(t, "a"))
		assert.Equal(t, 1, reader.readCount())
	})

	t.Run("no tenant extension", func(t *testing.T) {
		reader := newFakeReader()
		h := newTenantHarness(t, reader, TenantCacheConfig{}, define("operator", &recorder{}))
		assert.Nil(t, h.host.cache.Load())
		require.NoError(t, h.generate(t, "a"))
		assert.Zero(t, reader.readCount())
	})

	t.Run("not connected", func(t *testing.T) {
		log := &journal{}
		h := newHarness(t, configurable(t, "on", log, defaultOn), configurable(t, "off", log))
		require.NoError(t, h.generate(t, "a"))
		assert.Equal(t, []string{`on{}`}, transforms(log), "without API access tenant extensions keep their defaults")
	})
}

func TestBuildSnapshot(t *testing.T) {
	missingEnabled := TenantResource{Name: "t1", Type: "t1", Config: json.RawMessage(`{}`)}
	mismatch := stored("t1", true, `{}`)
	mismatch.Type = "other"
	for _, tc := range []struct {
		name      string
		resources []TenantResource
		ran       []string
		failed    float64
		refused   bool
	}{
		{name: "no resource uses the default", ran: []string{`on{}`}},
		{name: "enabled with a valid configuration", resources: []TenantResource{stored("t1", true, `{"v":"x"}`)}, ran: []string{`on{}`, `t1{"v":"x"}`}},
		{name: "disabled", resources: []TenantResource{stored("on", false, `{}`)}},
		{name: "configuration the schema refuses", resources: []TenantResource{stored("t1", true, `{"v":"`+tenantSentinel+`","extra":1}`)}, ran: []string{`on{}`}, failed: 1},
		{name: "missing enabled", resources: []TenantResource{missingEnabled}, ran: []string{`on{}`}, failed: 1},
		{name: "name differs from type", resources: []TenantResource{mismatch}, ran: []string{`on{}`}, failed: 1},
		{name: "fail-closed configuration refused", resources: []TenantResource{stored("gate", true, `{"v":1}`)}, ran: nil, failed: 1, refused: true},
		{name: "schema-less extension", resources: []TenantResource{stored("plain", true, `{}`)}, ran: []string{`on{}`, `plain{}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				reader := newFakeReader()
				reader.set("a", tc.resources...)
				log := &journal{}
				h := newTenantHarness(t, reader, TenantCacheConfig{},
					configurable(t, "on", log, defaultOn),
					configurable(t, "t1", log),
					configurable(t, "gate", log, failClosed),
					configurable(t, "plain", log, func(d *extension.Definition) { d.ConfigSchema = schema.Schema{} }),
				)
				err := h.generate(t, "a")
				if tc.refused {
					require.ErrorIs(t, err, extension.ErrUnavailable)
				} else {
					require.NoError(t, err)
				}
				assert.Equal(t, tc.ran, transforms(log))
				failures := 0.0
				for _, name := range []string{"t1", "gate"} {
					failures += h.events(name, string(phaseConfigure), eventFailed)
				}
				assert.Equal(t, tc.failed, failures)
				assert.NotContains(t, h.logs.String(), tenantSentinel)
			})
		})
	}
}

func TestTenantCache_ExactNumbersReachTheFactory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("t1", true, `{"n":9007199254740993}`))
		log := &journal{}
		h := newTenantHarness(t, reader, TenantCacheConfig{}, configurable(t, "t1", log))
		require.NoError(t, h.generate(t, "a"))
		assert.Equal(t, []string{`t1{"n":9007199254740993}`}, transforms(log))
	})
}

func TestTenantCache_UnknownTypes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("gone", true, `{}`), stored("retired", false, `{}`), stored("operator", true, `{}`))
		log := &journal{}
		h := newTenantHarness(t, reader, TenantCacheConfig{TTL: time.Minute}, configurable(t, "t1", log), define("operator", &recorder{}))
		for range 3 {
			require.NoError(t, h.generate(t, "a"))
		}
		for range 2 {
			time.Sleep(time.Minute)
			require.NoError(t, h.generate(t, "a"))
			synctest.Wait()
		}
		assert.Equal(t, 3, reader.readCount())
		assert.Equal(t, 2.0, metricValue(h, "grafana_ai_gateway_tenant_config_unknown_types_total", nil), "enabled unknown and operator types, once each")
		logs := h.logs.String()
		assert.Equal(t, 1, strings.Count(logs, `"type":"gone"`))
		assert.Equal(t, 1, strings.Count(logs, `"type":"operator"`))
		assert.NotContains(t, logs, "retired", "disabled unknown types are neither logged nor counted")
	})
}

func TestTenantCache_UnchangedRefreshKeepsInstances(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("t1", true, `{"v":"x"}`))
		var factoryRuns int
		var mu sync.Mutex
		definition := configurable(t, "t1", &journal{}, func(d *extension.Definition) {
			d.New = func(json.RawMessage, extension.Dependencies) (any, error) {
				mu.Lock()
				factoryRuns++
				mu.Unlock()
				return &recorder{}, nil
			}
		})
		h := newTenantHarness(t, reader, TenantCacheConfig{TTL: time.Minute}, definition)
		require.NoError(t, h.generate(t, "a"))
		time.Sleep(time.Minute)
		require.NoError(t, h.generate(t, "a"))
		synctest.Wait()
		assert.Equal(t, 2, reader.readCount())
		mu.Lock()
		assert.Equal(t, 1, factoryRuns)
		mu.Unlock()
	})
}

// closing is a tenant extension instance that counts its closes.
type closing struct {
	recorder
	mu     sync.Mutex
	closes int
}

func (c *closing) Close(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closes++
	return nil
}

func (c *closing) closeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closes
}

// closingExtension returns a tenant extension that builds a closing
// instance per configuration, recorded in built by configuration.
func closingExtension(t *testing.T, built *sync.Map) extension.Definition {
	t.Helper()
	return configurable(t, "t1", &journal{}, func(d *extension.Definition) {
		d.New = func(config json.RawMessage, _ extension.Dependencies) (any, error) {
			instance := &closing{}
			built.Store(string(config), instance)
			return instance, nil
		}
	})
}

func loadClosing(t *testing.T, built *sync.Map, config string) *closing {
	t.Helper()
	value, ok := built.Load(config)
	require.True(t, ok, config)
	return value.(*closing)
}

func TestTenantCache_ReplacementClosesAfterTheStreamAndObservation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("t1", true, `{"v":"old"}`))
		built := &sync.Map{}
		observed := make(chan struct{})
		h := newTenantHarness(t, reader, TenantCacheConfig{TTL: time.Minute}, closingExtension(t, built), define("watcher", &recorder{observe: func(context.Context, *extension.Call, extension.Record) error {
			<-observed
			return nil
		}}))
		parts := make(chan provider.StreamPart)
		h.model.stream = func(context.Context) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: parts}, nil
		}
		result, err := h.compose(t, extension.SourceOperatorCatalog).DoStream(inNamespace(t.Context(), "a"), provider.CallOptions{})
		require.NoError(t, err)
		old := loadClosing(t, built, `{"v":"old"}`)

		h.model.stream = nil
		reader.set("a", stored("t1", true, `{"v":"new"}`))
		time.Sleep(time.Minute)
		require.NoError(t, h.generate(t, "a"))
		synctest.Wait()
		require.NoError(t, h.generate(t, "a"))
		synctest.Wait()
		assert.Zero(t, old.closeCount(), "the open stream still holds the old instance")

		for _, part := range textParts("x") {
			parts <- part
		}
		close(parts)
		collect(t, result.Stream)
		synctest.Wait()
		assert.Zero(t, old.closeCount(), "the stream's observation still holds it")

		close(observed)
		synctest.Wait()
		assert.Equal(t, 1, old.closeCount())
		assert.Zero(t, loadClosing(t, built, `{"v":"new"}`).closeCount())
	})
}

func TestTenantCache_EvictionClosesAfterRelease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("t1", true, `{"v":"a"}`))
		reader.set("b", stored("t1", true, `{"v":"b"}`))
		built := &sync.Map{}
		release := make(chan struct{})
		h := newTenantHarness(t, reader, TenantCacheConfig{MaxNamespaces: 1}, closingExtension(t, built))
		h.model.generate = func(ctx context.Context) (*provider.GenerateResult, error) {
			if ctx.Value(namespaceKey{}) == "a" {
				<-release
			}
			return textResult("x"), nil
		}
		done := make(chan error, 1)
		go func() { done <- h.generate(t, "a") }()
		synctest.Wait()
		first := loadClosing(t, built, `{"v":"a"}`)

		require.NoError(t, h.generate(t, "b"))
		synctest.Wait()
		assert.Zero(t, first.closeCount(), "the evicted configuration is still in use")
		close(release)
		require.NoError(t, <-done)
		synctest.Wait()
		assert.Equal(t, 1, first.closeCount())

		require.NoError(t, h.generate(t, "a"))
		assert.Equal(t, 3, reader.readCount(), "an evicted namespace reads again")
		synctest.Wait()
		assert.Equal(t, 1, loadClosing(t, built, `{"v":"b"}`).closeCount(), "reading a again evicted b")
	})
}

func TestTenantCache_ShutdownClosesNamespaceInstances(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("t1", true, `{"v":"a"}`))
		reader.set("b", stored("t1", true, `{"v":"b"}`))
		built := &sync.Map{}
		h := newTenantHarness(t, reader, TenantCacheConfig{}, closingExtension(t, built))
		require.NoError(t, h.generate(t, "a"))
		require.NoError(t, h.generate(t, "b"))
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		h.host.Close(ctx)
		assert.Equal(t, 1, loadClosing(t, built, `{"v":"a"}`).closeCount())
		assert.Equal(t, 1, loadClosing(t, built, `{"v":"b"}`).closeCount())
		require.ErrorIs(t, h.generate(t, "c"), extension.ErrUnavailable)
	})
}

func TestTenantCache_SharedCollectors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("shared", true, `{}`), stored("fresh", true, `{}`))
		reader.set("b", stored("shared", true, `{}`), stored("fresh", true, `{}`))
		counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "shared_calls_total", Help: "Calls."})
		shared := configurable(t, "shared", &journal{}, func(d *extension.Definition) {
			d.ConfigSchema = schema.Schema{}
			d.New = func(_ json.RawMessage, deps extension.Dependencies) (any, error) {
				if err := deps.Registerer.Register(counter); err != nil {
					return nil, err
				}
				return &recorder{transform: func(_ context.Context, _ *extension.Call, params provider.CallOptions) (provider.CallOptions, error) {
					counter.Inc()
					return params, nil
				}}, nil
			}
		})
		log := &journal{}
		fresh := configurable(t, "fresh", log, func(d *extension.Definition) {
			d.ConfigSchema = schema.Schema{}
			d.New = func(_ json.RawMessage, deps extension.Dependencies) (any, error) {
				if err := deps.Registerer.Register(prometheus.NewCounter(prometheus.CounterOpts{Name: "fresh_total", Help: "Fresh."})); err != nil {
					return nil, err
				}
				return &recorder{name: "fresh", journal: log}, nil
			}
		})
		h := newTenantHarness(t, reader, TenantCacheConfig{}, shared, fresh)
		require.NoError(t, h.generate(t, "a"))
		require.NoError(t, h.generate(t, "b"))
		assert.Equal(t, 2.0, metricValue(h, "shared_calls_total", map[string]string{"extension": "shared"}))
		assert.Zero(t, h.events("shared", string(phaseConfigure), eventFailed))
		assert.Equal(t, []string{"fresh"}, transforms(log), "the second namespace's fresh collector fails")
		assert.Equal(t, 1.0, h.events("fresh", string(phaseConfigure), eventFailed))
	})
}

func TestReadClass(t *testing.T) {
	for err, class := range map[error]string{
		ErrReadRefused:           "refused",
		ErrReadNotFound:          "not-found",
		ErrReadStatus:            "unexpected-status",
		ErrReadTimeout:           "timeout",
		context.DeadlineExceeded: "timeout",
		ErrReadInvalid:           "invalid-response",
		errors.New("other"):      "error",
	} {
		assert.Equal(t, class, readClass(err))
	}
}

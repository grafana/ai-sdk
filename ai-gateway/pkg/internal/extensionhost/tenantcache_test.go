package extensionhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
// waits for a receive on the gate or, unless stubborn, for its context to end.
type fakeReader struct {
	mu       sync.Mutex
	stored   map[string][]TenantResource
	err      error
	gate     chan struct{}
	stubborn bool
	reads    []string
	contexts []context.Context
}

func newFakeReader() *fakeReader { return &fakeReader{stored: map[string][]TenantResource{}} }

func (r *fakeReader) Read(ctx context.Context, namespace string) ([]TenantResource, error) {
	r.mu.Lock()
	r.reads = append(r.reads, namespace)
	r.contexts = append(r.contexts, ctx)
	gate, stubborn := r.gate, r.stubborn
	r.mu.Unlock()
	switch {
	case gate != nil && stubborn:
		<-gate
	case gate != nil:
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

// stored returns a resource of type name with member config, or without a
// member when config is empty.
func stored(name string, enabled bool, config string) TenantResource {
	resource := TenantResource{Name: name, Type: name, Enabled: enabled}
	if config != "" {
		resource.Config = json.RawMessage(config)
	}
	return resource
}

func mustSchema(t *testing.T, raw string) schema.Schema {
	t.Helper()
	compiled, err := schema.SchemaFromJSON(json.RawMessage(raw))
	require.NoError(t, err)
	return compiled
}

// configRecorder is a tenant extension instance whose transform journals its
// name and the call's configuration.
type configRecorder struct {
	name    string
	journal *journal
}

func (r configRecorder) TransformRequest(_ context.Context, call *extension.Call, params provider.CallOptions) (provider.CallOptions, error) {
	r.journal.add(r.name + rendered(call.Config) + ":transform")
	return params, nil
}

// rendered renders a call's configuration for the journal: nil as empty,
// raw JSON as written.
func rendered(config any) string {
	switch value := config.(type) {
	case nil:
		return ""
	case json.RawMessage:
		return string(value)
	default:
		return fmt.Sprint(value)
	}
}

// configurable returns a default-off, skip tenant extension whose transform
// journals its name and the call's configuration.
func configurable(t *testing.T, name string, log *journal, change ...func(*extension.Definition)) extension.Definition {
	t.Helper()
	valueSchema := mustSchema(t, `{"type":"object","properties":{"v":{"type":"string"},"n":{"type":"integer"},"limits":{"type":"object","properties":{"rpm":{"type":"integer"}},"additionalProperties":false}},"additionalProperties":false}`)
	base := []func(*extension.Definition){tenant, skip, func(d *extension.Definition) {
		d.EnabledByDefault = false
		d.ConfigSchema = valueSchema
		d.New = func(json.RawMessage, extension.Dependencies) (any, error) {
			return configRecorder{name: name, journal: log}, nil
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

func TestBuildNamespace(t *testing.T) {
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
		{name: "invalid spec", resources: []TenantResource{{Name: "t1", Invalid: true}}, ran: []string{`on{}`}, failed: 1},
		{name: "invalid spec of a disabled default-on extension", resources: []TenantResource{{Name: "on", Invalid: true}}, failed: 1},
		{name: "enabled without a member", resources: []TenantResource{stored("t1", true, "")}, ran: []string{`on{}`}, failed: 1},
		{name: "disabled without a member", resources: []TenantResource{stored("t1", false, "")}, ran: []string{`on{}`}},
		{name: "nested member", resources: []TenantResource{stored("t1", true, `{"limits":{"rpm":10},"v":"x"}`)}, ran: []string{`on{}`, `t1{"limits":{"rpm":10},"v":"x"}`}},
		{name: "member for a schema-less extension", resources: []TenantResource{stored("plain", true, `{}`)}, ran: []string{`on{}`}, failed: 1},
		{name: "name differs from type", resources: []TenantResource{mismatch}, ran: []string{`on{}`}, failed: 1},
		{name: "fail-closed configuration refused", resources: []TenantResource{stored("gate", true, `{"v":1}`)}, ran: nil, failed: 1, refused: true},
		{name: "fail-closed decoding panics", resources: []TenantResource{stored("panics", true, `{}`)}, failed: 1, refused: true},
		{name: "decoding fails", resources: []TenantResource{stored("decodes", true, `{"v":"`+tenantSentinel+`"}`)}, ran: []string{`on{}`}, failed: 1},
		{name: "decoded value", resources: []TenantResource{stored("decodes", true, `{"v":"x"}`)}, ran: []string{`decodes{x}`, `on{}`}},
		{name: "schema-less extension", resources: []TenantResource{stored("plain", true, "")}, ran: []string{`on{}`, `plain`}},
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
					configurable(t, "panics", log, failClosed, func(d *extension.Definition) {
						d.Decode = func(json.RawMessage) (any, error) { panic(tenantSentinel) }
					}),
					configurable(t, "decodes", log, func(d *extension.Definition) { d.Decode = decodeValue }),
				)
				err := h.generate(t, "a")
				if tc.refused {
					require.ErrorIs(t, err, extension.ErrUnavailable)
					assert.Zero(t, h.model.callCount(), "refused before any provider call")
				} else {
					require.NoError(t, err)
				}
				assert.Equal(t, tc.ran, transforms(log))
				failures := 0.0
				for _, name := range []string{"on", "t1", "gate", "plain", "panics", "decodes"} {
					failures += h.events(name, string(phaseConfigure), eventFailed)
				}
				assert.Equal(t, tc.failed, failures)
				assert.NotContains(t, h.logs.String(), tenantSentinel)
			})
		})
	}
}

// decodedValue is a typed tenant configuration.
type decodedValue struct{ V string }

func (v *decodedValue) String() string { return "{" + v.V + "}" }

// decodeValue decodes a configuration's v field, refusing the sentinel.
func decodeValue(config json.RawMessage) (any, error) {
	var value decodedValue
	if err := json.Unmarshal(config, &value); err != nil {
		return nil, err
	}
	if value.V == tenantSentinel {
		return nil, errors.New(tenantSentinel)
	}
	return &value, nil
}

func TestTenantCache_ExactNumbersReachTheCall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("t1", true, `{"n":9007199254740993}`))
		log := &journal{}
		h := newTenantHarness(t, reader, TenantCacheConfig{}, configurable(t, "t1", log))
		require.NoError(t, h.generate(t, "a"))
		assert.Equal(t, []string{`t1{"n":9007199254740993}`}, transforms(log))
	})
}

func TestTenantCache_TypesNotImplemented(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("gone", true, ""), stored("retired", false, ""), stored("operator", true, ""), TenantResource{Name: "broken", Invalid: true})
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
		assert.Equal(t, 2.0, metricValue(h, "grafana_ai_gateway_tenant_config_unimplemented_types_total", nil), "enabled types not implemented, including an operator type, once each")
		logs := h.logs.String()
		assert.Equal(t, 1, strings.Count(logs, `"type":"gone"`))
		assert.Equal(t, 1, strings.Count(logs, `"type":"operator"`))
		assert.NotContains(t, logs, "retired", "disabled and invalid resources are neither logged nor counted")
		assert.NotContains(t, logs, "broken")
	})
}

func TestTenantCache_DecodesOncePerChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("t1", true, `{"v":"x"}`))
		reader.set("b", stored("t1", true, `{"v":"x"}`))
		var factoryRuns, decodes atomic.Int32
		definition := configurable(t, "t1", &journal{}, func(d *extension.Definition) {
			d.New = func(json.RawMessage, extension.Dependencies) (any, error) {
				factoryRuns.Add(1)
				return &recorder{}, nil
			}
			d.Decode = func(config json.RawMessage) (any, error) {
				decodes.Add(1)
				return decodeValue(config)
			}
		})
		h := newTenantHarness(t, reader, TenantCacheConfig{TTL: time.Minute}, definition)
		for range 3 {
			require.NoError(t, h.generate(t, "a"))
		}
		require.NoError(t, h.generate(t, "b"))
		assert.Equal(t, int32(2), decodes.Load(), "once per namespace read, not per call")
		time.Sleep(time.Minute)
		require.NoError(t, h.generate(t, "a"))
		synctest.Wait()
		assert.Equal(t, int32(2), decodes.Load(), "an unchanged refresh decodes nothing")
		reader.set("a", stored("t1", true, `{"v":"y"}`))
		time.Sleep(time.Minute)
		require.NoError(t, h.generate(t, "a"))
		synctest.Wait()
		assert.Equal(t, int32(3), decodes.Load())
		assert.Equal(t, int32(1), factoryRuns.Load(), "the factory runs once per process")
	})
}

func TestTenantCache_CallConfig(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("typed", true, `{"v":"x"}`), stored("raw", true, `{"v":"y"}`))
		var mu sync.Mutex
		seen := map[string][]any{}
		record := func(name string) *recorder {
			return &recorder{admit: func(_ context.Context, call *extension.Call) error {
				mu.Lock()
				defer mu.Unlock()
				seen[name] = append(seen[name], call.Config)
				return nil
			}}
		}
		typed := configurable(t, "typed", &journal{}, func(d *extension.Definition) {
			d.Decode = decodeValue
			d.New = func(json.RawMessage, extension.Dependencies) (any, error) { return record("typed"), nil }
		})
		raw := configurable(t, "raw", &journal{}, func(d *extension.Definition) {
			d.New = func(json.RawMessage, extension.Dependencies) (any, error) { return record("raw"), nil }
		})
		h := newTenantHarness(t, reader, TenantCacheConfig{}, typed, raw, define("operator", record("operator")))
		for range 2 {
			require.NoError(t, h.generate(t, "a"))
		}
		mu.Lock()
		defer mu.Unlock()
		require.Len(t, seen["typed"], 2)
		assert.Equal(t, &decodedValue{V: "x"}, seen["typed"][0])
		assert.Same(t, seen["typed"][0], seen["typed"][1], "calls share the namespace's decoded value")
		assert.Equal(t, []any{json.RawMessage(`{"v":"y"}`), json.RawMessage(`{"v":"y"}`)}, seen["raw"], "without Decode, calls get the member as read")
		assert.Equal(t, []any{nil, nil}, seen["operator"])
	})
}

// closing is a tenant extension instance that counts its closes and journals
// the configuration each response part sees.
type closing struct {
	recorder
	mu     sync.Mutex
	closes int
	parts  []string
}

func (c *closing) InspectResponse(_ context.Context, call *extension.Call, _ extension.Part) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.parts = append(c.parts, rendered(call.Config))
	return nil
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

func (c *closing) seen() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Compact(slices.Clone(c.parts))
}

// closingExtension returns a tenant extension with one closing instance.
func closingExtension(t *testing.T, instance *closing) extension.Definition {
	t.Helper()
	return configurable(t, "t1", &journal{}, func(d *extension.Definition) {
		d.New = func(json.RawMessage, extension.Dependencies) (any, error) { return instance, nil }
	})
}

func TestTenantCache_CallsKeepTheirConfigurationAcrossChanges(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("t1", true, `{"v":"old"}`))
		reader.set("b", stored("t1", true, `{"v":"b"}`))
		instance := &closing{}
		h := newTenantHarness(t, reader, TenantCacheConfig{TTL: time.Minute, MaxNamespaces: 1}, closingExtension(t, instance))
		parts := make(chan provider.StreamPart)
		h.model.stream = func(context.Context) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: parts}, nil
		}
		result, err := h.compose(t, extension.SourceOperatorCatalog).DoStream(inNamespace(t.Context(), "a"), provider.CallOptions{})
		require.NoError(t, err)
		h.model.stream = nil

		reader.set("a", stored("t1", true, `{"v":"new"}`))
		time.Sleep(time.Minute)
		require.NoError(t, h.generate(t, "a"))
		synctest.Wait()
		require.NoError(t, h.generate(t, "a"))
		require.NoError(t, h.generate(t, "b"), "evicts a while its stream is open")
		synctest.Wait()

		for _, part := range textParts("x") {
			parts <- part
		}
		close(parts)
		collect(t, result.Stream)
		synctest.Wait()
		assert.Equal(t, []string{`{"v":"old"}`, `{"v":"new"}`, `{"v":"b"}`, `{"v":"old"}`}, instance.seen(), "the refresh call is served the old configuration, and the open stream keeps the one it started with")
		assert.Zero(t, instance.closeCount(), "neither a refresh nor an eviction closes an instance")

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		h.host.Close(ctx)
		h.host.Close(ctx)
		assert.Equal(t, 1, instance.closeCount(), "the instance is closed once, at shutdown")
		require.ErrorIs(t, h.generate(t, "c"), extension.ErrUnavailable)
	})
}

func TestTenantCache_CollectorsRegisterOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("counted", true, ""))
		reader.set("b", stored("counted", true, ""))
		counted := configurable(t, "counted", &journal{}, func(d *extension.Definition) {
			d.ConfigSchema = schema.Schema{}
			d.New = func(_ json.RawMessage, deps extension.Dependencies) (any, error) {
				counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "counted_calls_total", Help: "Calls."})
				if err := deps.Registerer.Register(counter); err != nil {
					return nil, err
				}
				return &recorder{transform: func(_ context.Context, _ *extension.Call, params provider.CallOptions) (provider.CallOptions, error) {
					counter.Inc()
					return params, nil
				}}, nil
			}
		})
		h := newTenantHarness(t, reader, TenantCacheConfig{}, counted)
		require.NoError(t, h.generate(t, "a"))
		require.NoError(t, h.generate(t, "b"))
		assert.Equal(t, 2.0, metricValue(h, "counted_calls_total", map[string]string{"extension": "counted"}), "a collector created in New registers once, and both namespaces use it")
		assert.Zero(t, h.events("counted", string(phaseConfigure), eventFailed))
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

func TestTenantCache_IdleNamespaceKeepsItsConfiguration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("t1", true, `{"v":"x"}`))
		var decodes atomic.Int32
		log := &journal{}
		definition := configurable(t, "t1", log, failClosed, func(d *extension.Definition) {
			d.Decode = func(config json.RawMessage) (any, error) {
				decodes.Add(1)
				return config, nil
			}
		})
		h := newTenantHarness(t, reader, TenantCacheConfig{TTL: time.Minute, MaxStaleness: 10 * time.Minute}, definition)
		require.NoError(t, h.generate(t, "a"))
		time.Sleep(3 * time.Hour)
		require.NoError(t, h.generate(t, "a"), "a namespace idle past the staleness bound is not refused")
		synctest.Wait()
		require.NoError(t, h.generate(t, "a"))
		assert.Equal(t, []string{`t1{"v":"x"}`, `t1{"v":"x"}`, `t1{"v":"x"}`}, transforms(log))
		assert.Equal(t, int32(1), decodes.Load(), "an unchanged refresh after idling decodes nothing")
		assert.Equal(t, 2, reader.readCount())
	})
}

func TestTenantCache_NewEntryIsNotEvictedByLoadsInFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		gate := make(chan struct{})
		reader.setGate(gate)
		log := &journal{}
		h := newTenantHarness(t, reader, TenantCacheConfig{MaxNamespaces: 1}, configurable(t, "t1", log))

		var calls sync.WaitGroup
		calls.Go(func() { assert.NoError(t, h.generate(t, "a")) })
		synctest.Wait()
		for range 3 {
			calls.Go(func() { assert.NoError(t, h.generate(t, "b")) })
		}
		synctest.Wait()
		assert.Equal(t, 2, reader.readCount(), "concurrent first calls for b share one read while a's is in flight")
		close(gate)
		calls.Wait()
		assert.Equal(t, 2, reader.readCount())
	})
}

func TestTenantCache_ShutdownWaitsForReadsInFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		gate := make(chan struct{})
		reader.setGate(gate)
		reader.stubborn = true
		reader.set("a", stored("t1", true, `{"v":"a"}`))
		var decoded atomic.Bool
		definition := configurable(t, "t1", &journal{}, func(d *extension.Definition) {
			d.Decode = func(config json.RawMessage) (any, error) {
				decoded.Store(true)
				return config, nil
			}
		})
		h := newTenantHarness(t, reader, TenantCacheConfig{}, definition)
		ctx, cancel := context.WithCancel(inNamespace(t.Context(), "a"))
		defer cancel()
		go func() { _, _ = h.compose(t, extension.SourceOperatorCatalog).DoGenerate(ctx, provider.CallOptions{}) }()
		synctest.Wait()

		closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
		defer closeCancel()
		closed := make(chan struct{})
		go func() {
			h.host.Close(closeCtx)
			close(closed)
		}()
		synctest.Wait()
		select {
		case <-closed:
			t.Fatal("Close returned while a read was in flight")
		default:
		}
		close(gate)
		<-closed
		synctest.Wait()
		assert.False(t, decoded.Load(), "a read that completes during shutdown decodes nothing")
		assert.NotContains(t, h.logs.String(), "gateway extension failed")
	})
}

// phaselessInstance implements no phase, only Close.
type phaselessInstance struct{ closes atomic.Int32 }

func (p *phaselessInstance) Close(context.Context) error {
	p.closes.Add(1)
	return nil
}

func TestNew_TenantExtensions(t *testing.T) {
	newTenantHost := func(t *testing.T, definitions ...extension.Definition) (*Host, error) {
		t.Helper()
		require.NoError(t, ValidateDefinitions(definitions, nil, typesOf(definitions)))
		plan, err := NewPlan(definitions, nil, nil)
		require.NoError(t, err)
		return New(Config{Plan: plan, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Registerer: prometheus.NewRegistry(), Clock: time.Now, RequestInfo: func(context.Context) extension.Call { return extension.Call{} }})
	}

	t.Run("every tenant extension is constructed once", func(t *testing.T) {
		var runs atomic.Int32
		count := func(d *extension.Definition) {
			d.New = func(config json.RawMessage, _ extension.Dependencies) (any, error) {
				runs.Add(1)
				assert.JSONEq(t, `{}`, string(config), "a tenant factory gets an empty configuration")
				return &recorder{}, nil
			}
		}
		host, err := newTenantHost(t, configurable(t, "off", &journal{}, count), configurable(t, "on", &journal{}, defaultOn, count))
		require.NoError(t, err)
		t.Cleanup(func() { host.Close(context.Background()) })
		assert.Equal(t, int32(2), runs.Load())
	})

	t.Run("default configuration decoded once", func(t *testing.T) {
		var decodes atomic.Int32
		host, err := newTenantHost(t, configurable(t, "on", &journal{}, defaultOn, func(d *extension.Definition) {
			d.Decode = func(config json.RawMessage) (any, error) {
				decodes.Add(1)
				return decodeValue(config)
			}
		}))
		require.NoError(t, err)
		t.Cleanup(func() { host.Close(context.Background()) })
		assert.Equal(t, int32(1), decodes.Load())
		assert.Equal(t, []any{&decodedValue{}}, host.lists[extension.KeysOperator].configs)
	})

	t.Run("default configuration that fails to decode", func(t *testing.T) {
		_, err := newTenantHost(t, configurable(t, "on", &journal{}, defaultOn, func(d *extension.Definition) {
			d.Decode = func(json.RawMessage) (any, error) { return nil, errors.New(tenantSentinel) }
		}))
		require.EqualError(t, err, `extensions: decoding the default configuration of extension "on" failed`)
	})

	t.Run("tenant instance without a phase", func(t *testing.T) {
		instance := &phaselessInstance{}
		_, err := newTenantHost(t, configurable(t, "phaseless", &journal{}, func(d *extension.Definition) {
			d.New = func(json.RawMessage, extension.Dependencies) (any, error) { return instance, nil }
		}))
		require.EqualError(t, err, `extensions: extension "phaseless" implements no phase`)
		assert.Equal(t, int32(1), instance.closes.Load(), "the instance built is closed")
	})
}

func TestTenantCache_RemovedResourceRestoresTheDefault(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.set("a", stored("on", true, `{"v":"x"}`))
		log := &journal{}
		h := newTenantHarness(t, reader, TenantCacheConfig{TTL: time.Minute}, configurable(t, "on", log, defaultOn))
		require.NoError(t, h.generate(t, "a"))
		reader.set("a")
		time.Sleep(time.Minute)
		require.NoError(t, h.generate(t, "a"))
		synctest.Wait()
		require.NoError(t, h.generate(t, "a"))
		assert.Equal(t, []string{`on{"v":"x"}`, `on{"v":"x"}`, `on{}`}, transforms(log))
	})
}

func TestTenantCache_ReadFailureLogs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.fail(ErrReadRefused)
		h := newTenantHarness(t, reader, TenantCacheConfig{}, configurable(t, "t1", &journal{}))
		require.NoError(t, h.generate(t, "stacks-9"))
		var record map[string]any
		for line := range strings.Lines(h.logs.String()) {
			if strings.Contains(line, "gateway tenant configuration read failed") {
				require.NoError(t, json.Unmarshal([]byte(line), &record))
			}
		}
		require.NotNil(t, record)
		assert.Equal(t, "stacks-9", record["namespace"])
		assert.Equal(t, "refused", record["class"])
		assert.ElementsMatch(t, []string{"time", "level", "msg", "namespace", "class"}, keys(record))
	})
}

func TestTenantCache_ShutdownCancelsReadsWithoutCountingThem(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		reader.setGate(make(chan struct{}))
		h := newTenantHarness(t, reader, TenantCacheConfig{}, configurable(t, "t1", &journal{}))
		ctx, cancel := context.WithCancel(inNamespace(t.Context(), "a"))
		defer cancel()
		go func() { _, _ = h.compose(t, extension.SourceOperatorCatalog).DoGenerate(ctx, provider.CallOptions{}) }()
		synctest.Wait()
		closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
		defer closeCancel()
		h.host.Close(closeCtx)
		assert.Zero(t, metricValue(h, "grafana_ai_gateway_tenant_config_reads_total", map[string]string{"result": "failed"}))
		assert.NotContains(t, h.logs.String(), "read failed")
	})
}

func keys(record map[string]any) []string {
	names := make([]string, 0, len(record))
	for name := range record {
		names = append(names, name)
	}
	return names
}

func TestTenantCache_ConnectedAfterShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := newFakeReader()
		h := newHarnessWith(t, func(config *Config) {
			config.RequestInfo = func(ctx context.Context) extension.Call {
				namespace, _ := ctx.Value(namespaceKey{}).(string)
				return extension.Call{Namespace: namespace}
			}
		}, configurable(t, "t1", &journal{}))
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		h.host.Close(ctx)
		h.host.ConnectTenants(reader)
		require.ErrorIs(t, h.generate(t, "a"), extension.ErrUnavailable, "a cache connected after shutdown starts closed")
		assert.Zero(t, reader.readCount())
	})
}

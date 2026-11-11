package extensionhost

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type valueCollector struct{ desc *prometheus.Desc }

func (c valueCollector) Describe(descriptions chan<- *prometheus.Desc) { descriptions <- c.desc }
func (c valueCollector) Collect(metrics chan<- prometheus.Metric) {
	metrics <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, 1)
}

func TestSharedRegisterer(t *testing.T) {
	newCounter := func() prometheus.Counter {
		return prometheus.NewCounter(prometheus.CounterOpts{Name: "shared_total", Help: "Shared."})
	}

	t.Run("same collector registered twice", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		registerer := newSharedRegisterer(registry, "quota")
		counter := newCounter()
		require.NoError(t, registerer.Register(counter))
		require.NoError(t, registerer.Register(counter))
		assert.NotPanics(t, func() { registerer.MustRegister(counter) })
		counter.Inc()
		counter.Inc()
		assert.Equal(t, 2.0, counterValue(registry, "shared_total", map[string]string{"extension": "quota"}))
		families, err := registry.Gather()
		require.NoError(t, err)
		require.Len(t, families, 1)
		assert.Len(t, families[0].GetMetric(), 1)
	})

	t.Run("different collector with the same descriptors", func(t *testing.T) {
		registerer := newSharedRegisterer(prometheus.NewRegistry(), "quota")
		require.NoError(t, registerer.Register(newCounter()))
		var already prometheus.AlreadyRegisteredError
		assert.ErrorAs(t, registerer.Register(newCounter()), &already)
		assert.Panics(t, func() { registerer.MustRegister(newCounter()) })
	})

	t.Run("uncomparable collector type", func(t *testing.T) {
		registerer := newSharedRegisterer(prometheus.NewRegistry(), "quota")
		collector := valueCollector{desc: prometheus.NewDesc("value", "Value.", nil, nil)}
		require.NoError(t, registerer.Register(collector))
		assert.NotPanics(t, func() {
			var already prometheus.AlreadyRegisteredError
			assert.ErrorAs(t, registerer.Register(collector), &already)
		})
	})
}

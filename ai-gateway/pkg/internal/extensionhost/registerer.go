package extensionhost

import (
	"errors"
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
)

// sharedRegisterer is the registerer every instance of one definition
// receives. Registering a collector the definition already registered
// succeeds, so a factory that runs once per namespace configuration can
// register the collectors its definition created once. A different collector
// with the same descriptors still fails.
type sharedRegisterer struct {
	prometheus.Registerer
}

func newSharedRegisterer(registerer prometheus.Registerer, name string) sharedRegisterer {
	return sharedRegisterer{Registerer: prometheus.WrapRegistererWith(prometheus.Labels{"extension": name}, registerer)}
}

func (r sharedRegisterer) Register(collector prometheus.Collector) error {
	err := r.Registerer.Register(collector)
	var already prometheus.AlreadyRegisteredError
	if errors.As(err, &already) && sameCollector(already.ExistingCollector, collector) {
		return nil
	}
	return err
}

func (r sharedRegisterer) MustRegister(collectors ...prometheus.Collector) {
	for _, collector := range collectors {
		if err := r.Register(collector); err != nil {
			panic(err)
		}
	}
}

// sameCollector reports whether two collectors are the same pointer. Other
// collector types are never treated as the same, since comparing them can
// panic.
func sameCollector(a, b prometheus.Collector) bool {
	kind := reflect.TypeOf(a)
	return kind != nil && kind.Kind() == reflect.Pointer && kind == reflect.TypeOf(b) && a == b
}

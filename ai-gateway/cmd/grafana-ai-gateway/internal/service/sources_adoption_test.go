//go:build source_observer_adoption

package service

import "testing"

func TestSourcesPublishedObserverAdoption(t *testing.T) {
	testSourcesMetadataOnlyObservation(t, true)
}

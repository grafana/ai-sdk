package aisdk

import "testing"

var (
	_ Option         = sharedOption{}
	_ StreamOption   = sharedOption{}
	_ GenerateOption = sharedOption{}

	_ StreamOption = streamOnlyOption{}

	_ GenerateOption = generateOnlyOption{}
)

func TestOptionSatisfiesBothInterfaces(t *testing.T) {
	opt := WithTemperature(0.7)

	var _ StreamOption = opt
	var _ GenerateOption = opt
	_ = Option(opt)
}

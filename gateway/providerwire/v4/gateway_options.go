package providerwirev4

import "fmt"

func validateAndRemoveGatewayOptions(options providerOptionsDTO) (providerOptionsDTO, error) {
	gateway, exists := options["gateway"]
	if !exists {
		return options, nil
	}
	object, err := decodeObject(gateway, "gateway provider options")
	if err != nil {
		return nil, err
	}
	if len(object) != 0 {
		return nil, fmt.Errorf("providerwirev4: providerOptions.gateway is unsupported")
	}
	remaining := make(providerOptionsDTO, len(options)-1)
	for key, value := range options {
		if key != "gateway" {
			remaining[key] = value
		}
	}
	return remaining, nil
}

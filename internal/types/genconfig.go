package types

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// PrestepServiceConfig defines a mapping between the prestep pkg (which is essentially the
// unique identifier for a prestep) and config for that prestep. For example,
// github.com/play-with-go/gitea will map to an endpoint that explains where that
// prestep can be "found". The Networks value represents a (non-production) config
// that describes which Docker networks the request should be made within.
type PrestepServiceConfig map[string]*ServiceConfig

// ServiceConfig defines a URL endpoint where a prestep can be accessed. It
// also defines optional Docker networks to join when this service is accessed
// by preguide in a development mode.
type ServiceConfig struct {
	Endpoint *url.URL
	Networks []string
}

func (p *ServiceConfig) UnmarshalJSON(b []byte) error {
	var v struct {
		Endpoint string
		Networks []string
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("failed to unmarshal prestepConfig: %v", err)
	}
	u, err := url.Parse(v.Endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse URL from prestepConfig Endpoint %q: %v", v.Endpoint, err)
	}
	p.Endpoint = u
	p.Networks = v.Networks
	return nil
}

package types

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// genConfig defines a mapping between the prestep pkg (which is essentially the
// unique identifier for a prestep) and config for that prestep. For example,
// github.com/play-with-go/gitea will map to an endpoint that explains where that
// prestep can be "found". The Networks value represents a (non-production) config
// that describes which Docker networks the request should be made within.
type GenConfig map[string]*PrestepConfig

type PrestepConfig struct {
	Endpoint *url.URL
	Networks []string
}

func (p *PrestepConfig) UnmarshalJSON(b []byte) error {
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

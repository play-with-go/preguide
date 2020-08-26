package preguide

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/load"
	"github.com/play-with-go/preguide/internal/embed"
)

type PrestepOut struct {
	Vars []string
}

type GuideStructures map[string]*GuideStructure

type GuideStructure struct {
	Presteps  []*Prestep
	Terminals []*Terminal
	Scenarios []*Scenario
	Networks  []string
	Env       []string
}

type Prestep struct {
	Package string
	Path    string
	Args    interface{}
}

type Terminal struct {
	Name        string
	Description string
	Scenarios   map[string]*TerminalScenario
}

type TerminalScenario struct {
	Image string
}

type Scenario struct {
	Name        string
	Description string
}

type Schemas struct {
	PrestepServiceConfig cue.Value
	Guide                cue.Value
	Command              cue.Value
	CommandFile          cue.Value
	Upload               cue.Value
	UploadFile           cue.Value
	GuideOutput          cue.Value
	CommandStep          cue.Value
	UploadStep           cue.Value
	GuideStructures      cue.Value
}

func LoadSchemas(r *cue.Runtime) (res Schemas, err error) {
	defer func() {
		switch r := recover(); r {
		case nil, err:
		default:
			panic(r)
		}
	}()

	overlay := make(map[string]load.Source)
	for _, asset := range embed.AssetNames() {
		contents, err := embed.Asset(asset)
		if err != nil {
			panic(err)
		}
		overlay[filepath.Join("/", asset)] = load.FromBytes(contents)
	}
	conf := &load.Config{
		Dir:     "/",
		Overlay: overlay,
	}
	bps := load.Instances([]string{".", "./out"}, conf)
	preguide, err := r.Build(bps[0])
	if err != nil {
		return res, fmt.Errorf("failed to load github.com/play-with-go/preguide package: %v", err)
	}
	preguideOut, err := r.Build(bps[1])
	if err != nil {
		return res, fmt.Errorf("failed to load github.com/play-with-go/preguide/out package: %v", err)
	}

	mustFind := func(v cue.Value) cue.Value {
		if err = v.Err(); err != nil {
			panic(err)
		}
		return v
	}

	res.PrestepServiceConfig = mustFind(preguide.LookupDef("#PrestepServiceConfig"))
	res.Guide = mustFind(preguide.LookupDef("#Guide"))
	res.Command = mustFind(preguide.LookupDef("#Command"))
	res.CommandFile = mustFind(preguide.LookupDef("#CommandFile"))
	res.Upload = mustFind(preguide.LookupDef("#Upload"))
	res.UploadFile = mustFind(preguide.LookupDef("#UploadFile"))
	res.GuideOutput = mustFind(preguideOut.LookupDef("#GuideOutput"))
	res.CommandStep = mustFind(preguideOut.LookupDef("#CommandStep"))
	res.UploadStep = mustFind(preguideOut.LookupDef("#UploadStep"))
	res.GuideStructures = mustFind(preguideOut.LookupDef("#GuideStructures"))

	return res, nil
}

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
	Env      []string
	Networks []string
}

func (p *ServiceConfig) UnmarshalJSON(b []byte) error {
	var v struct {
		Endpoint string
		Env      []string
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
	p.Env = v.Env
	p.Networks = v.Networks
	return nil
}

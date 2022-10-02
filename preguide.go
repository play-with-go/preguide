// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package preguide

import (
	goembed "embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/load"
)

//go:embed preguide.cue out/out.cue cue.mod/module.cue
var assets goembed.FS

type PrestepOut struct {
	Vars []string
}

type GuideStructures map[string]*GuideStructure

type GuideStructure struct {
	Delims    [2]string
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
	Upload               cue.Value
	GuideOutput          cue.Value
	CommandStep          cue.Value
	UploadStep           cue.Value
	GuideStructures      cue.Value
}

func LoadSchemas(r *cue.Context) (res Schemas, err error) {
	defer func() {
		switch r := recover(); r {
		case nil, err:
		default:
			panic(r)
		}
	}()

	overlay := make(map[string]load.Source)
	fs.WalkDir(assets, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			panic(err) // this is seriously bad
		}
		if !d.Type().IsRegular() {
			return nil
		}
		contents, err := assets.ReadFile(path)
		if err != nil {
			panic(err) // this is seriously bad
		}
		overlay[filepath.Join("/", path)] = load.FromBytes(contents)
		return nil
	})
	conf := &load.Config{
		Dir:     "/",
		Overlay: overlay,
	}
	bps := load.Instances([]string{".", "./out"}, conf)
	preguide := r.BuildInstance(bps[0])
	if err := preguide.Err(); err != nil {
		return res, fmt.Errorf("failed to load github.com/play-with-go/preguide package: %v", err)
	}
	preguideOut := r.BuildInstance(bps[1])
	if err := preguideOut.Err(); err != nil {
		return res, fmt.Errorf("failed to load github.com/play-with-go/preguide/out package: %v", err)
	}

	mustFind := func(v cue.Value) cue.Value {
		if err = v.Err(); err != nil {
			panic(err)
		}
		return v
	}

	res.PrestepServiceConfig = mustFind(preguide.LookupPath(cue.ParsePath("#PrestepServiceConfig")))
	res.Guide = mustFind(preguide.LookupPath(cue.ParsePath("#Guide")))
	res.Command = mustFind(preguide.LookupPath(cue.ParsePath("#Command")))
	res.Upload = mustFind(preguide.LookupPath(cue.ParsePath("#Upload")))
	res.GuideOutput = mustFind(preguideOut.LookupPath(cue.ParsePath("#GuideOutput")))
	res.CommandStep = mustFind(preguideOut.LookupPath(cue.ParsePath("#CommandStep")))
	res.UploadStep = mustFind(preguideOut.LookupPath(cue.ParsePath("#UploadStep")))
	res.GuideStructures = mustFind(preguideOut.LookupPath(cue.ParsePath("#GuideStructures")))

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

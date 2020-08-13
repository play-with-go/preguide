package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type dockerCmd struct {
	fs           *flag.FlagSet
	flagDefaults string
}

func newDockerCmd() *dockerCmd {
	res := &dockerCmd{}
	res.flagDefaults = newFlagSet("preguide docker", func(fs *flag.FlagSet) {
		res.fs = fs
	})
	return res
}

func (i *dockerCmd) usage() string {
	return fmt.Sprintf(`
usage: preguide docker

%s`[1:], i.flagDefaults)
}

func (i *dockerCmd) usageErr(format string, args ...interface{}) usageErr {
	return usageErr{fmt.Errorf(format, args...), i}
}

func (r *runner) runDocker(args []string) error {
	// Usage:
	//
	//     preguide docker METHOD URL ARGS
	//
	// where ARGS is a JSON-encoded string. Returns (via stdout) the JSON-encoded result
	// (without checking that result)

	var body io.Reader

	switch len(args) {
	case 2:
	case 3:
		body = strings.NewReader(args[2])
	default:
		return r.dockerCmd.usageErr("expected either 2 or 3 args; got %v", len(args))
	}

	method, url := args[0], args[1]

	req, err := http.NewRequest(method, url, body)
	check(err, "failed to build a new request for a %v to %v: %v", method, url, err)

	resp, err := http.DefaultClient.Do(req)
	check(err, "failed to execute %v: %v", req, err)

	_, err = io.Copy(os.Stdout, resp.Body)
	check(err, "failed to read response body from %v: %v", req, err)

	return nil
}

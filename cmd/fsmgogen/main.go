package main

import (
	"flag"
	"fmt"
	"github.com/fsmgo/fsmgo/pkg/generator"
	"os"
	"strings"
)

var states = flag.String("states", "", "comma-separated list of states")
var events = flag.String("events", "", "comma-separated list of events")
var transitions = flag.String("transitions", "", "comma-separated list of from:event:to tuples")
var pkg = flag.String("package", "", "target package")
var notests = flag.Bool("notests", false, "do not generate tests")
var dir = flag.String("dir", ".", "target path to put generated files in")

func main() {
	flag.Parse()
	cfg, err := validate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err.Error())
		os.Exit(-1)
	}
	err = generator.Generate(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Generator error: %v\n", err.Error())
		os.Exit(-2)
	}
}

func validate() (*generator.Config, error) {
	rt := &generator.Config{}
	sSet := make(map[string]bool)
	eSet := make(map[string]bool)

	sts := strings.TrimSpace(*states)
	if sts == "" {
		return nil, fmt.Errorf("-states value required")
	}
	for _, s := range strings.Split(sts, ",") {
		ts := strings.TrimSpace(s)
		rt.States = append(rt.States, ts)
		sSet[ts] = true
	}

	evts := strings.TrimSpace(*events)
	if evts == "" {
		return nil, fmt.Errorf("-events value required")
	}
	for _, e := range strings.Split(evts, ",") {
		te := strings.TrimSpace(e)
		rt.Events = append(rt.Events, te)
		eSet[te] = true
	}

	trs := strings.TrimSpace(*transitions)
	if trs == "" {
		return nil, fmt.Errorf("-transitions value required")
	}
	for _, t := range strings.Split(trs, ",") {
		tt := strings.TrimSpace(t)
		tts := strings.Split(t, ":")
		if len(tts) != 3 {
			return nil, fmt.Errorf("invalid transition: %v", tt)
		}
		frm := strings.TrimSpace(tts[0])
		if _, ok := sSet[frm]; !ok {
			return nil, fmt.Errorf("transition %q: 'from' state %q is not in the states list", tt, frm)
		}
		ev := strings.TrimSpace(tts[1])
		if _, ok := eSet[ev]; !ok {
			return nil, fmt.Errorf("transition %q: event %q is not in the event list", tt, frm)
		}
		to := strings.TrimSpace(tts[2])
		if _, ok := sSet[to]; !ok {
			return nil, fmt.Errorf("transition %q: 'to' state %q is not in the states list", tt, to)
		}
		rt.Transitions = append(rt.Transitions, [...]string{frm, ev, to})
	}

	pkg := strings.TrimSpace(*pkg)
	if pkg == "" {
		return nil, fmt.Errorf("-package value required")
	}
	rt.Pkg = pkg
	rt.NoTests = *notests
	rt.Path = *dir
	return rt, nil
}

/*
 * Copyright 2022 goFSM authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package generator

import (
	_ "embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type StateParam struct {
	Package        string
	State          string
	CapState       string
	Events         []EventInfo
	Transitions    []transitionInfo
	TransitionsMap map[string]string
	CommonsHere    bool
}

type BaseParam struct {
	Package string
	Events  []EventInfo
	States  []string
}

//go:embed templates/states.tmpl
var statesTemplate string

//go:embed templates/events.tmpl
var eventsTemplate string

//go:embed templates/base.tmpl
var baseTemplate string

//go:embed templates/state.tmpl
var stateTemplate string

//go:embed templates/test.tmpl
var stateTestTemplate string

type EventInfo struct {
	Event    string
	CapEvent string
	Method   string
}

func convertCase(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.Title(s)
	return strings.ReplaceAll(s, " ", "")
}

func eventsToMethods(evs []string) []EventInfo {
	var rt []EventInfo
	for _, e := range evs {
		capEvent := convertCase(e)
		m := "On" + capEvent
		rt = append(rt, EventInfo{
			Event:    e,
			CapEvent: capEvent,
			Method:   m,
		})
	}
	return rt
}

func exec(fp string, t *template.Template, data any) error {
	f, err := os.Create(fp)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return t.Execute(f, data)
}

func has(ss []string, s string) bool {
	idx := sort.SearchStrings(ss, s)
	return idx < len(ss) && ss[idx] == s
}

type transitionInfo struct {
	OrigEvent string
	Event     string
	ToState   string
}

func validateTransitions(states []string, events []string, transitions [][3]string) (map[string][]transitionInfo, error) {
	stateTrans := make(map[string][]transitionInfo)
	// validate transitions
	for _, tr := range transitions {
		if !has(states, tr[0]) {
			return nil, fmt.Errorf("invalid 'from' state in transitin %s(%s)=>%s", tr[0], tr[1], tr[2])
		}
		if !has(states, tr[2]) {
			return nil, fmt.Errorf("invalid 'to' state in transitin %s(%s)=>%s", tr[0], tr[1], tr[2])
		}
		if !has(events, tr[1]) {
			return nil, fmt.Errorf("invalid event in transitin %s(%s)=>%s", tr[0], tr[1], tr[2])
		}
		stateTrans[tr[0]] = append(stateTrans[tr[0]], transitionInfo{
			OrigEvent: tr[1],
			Event:     convertCase(tr[1]),
			ToState:   convertCase(tr[2]),
		})
	}
	return stateTrans, nil
}

type Config struct {
	States      []string
	Events      []string
	Transitions [][3]string
	Pkg         string
	Path        string
	NoTests     bool
}

func Generate(cfg *Config) error {
	states, events, transitions := cfg.States, cfg.Events, cfg.Transitions

	sort.Strings(states)
	sort.Strings(events)

	stateTrans, err := validateTransitions(states, events, transitions)
	if err != nil {
		return err
	}

	statesTempl := template.Must(template.New("states").Parse(statesTemplate))
	eventsTempl := template.Must(template.New("events").Parse(eventsTemplate))
	baseTempl := template.Must(template.New("base").Parse(baseTemplate))
	stateTempl := template.Must(template.New("state").Parse(stateTemplate))
	stateTestTempl := template.Must(template.New("state_test").Parse(stateTestTemplate))

	b := &BaseParam{
		Package: cfg.Pkg,
		Events:  eventsToMethods(events),
	}
	if err := ensurePath(cfg.Path); err != nil {
		return err
	}

	needCommons := true
	for _, state := range states {
		lowState := strings.ToLower(state)
		if reserved(lowState) {
			return fmt.Errorf("%q state name is reserved", state)
		}
		st := convertCase(state)
		tm := make(map[string]string)
		for _, v := range stateTrans[state] {
			tm[v.OrigEvent] = v.ToState
		}
		b.States = append(b.States, st)
		s := &StateParam{
			Package:        cfg.Pkg,
			State:          strings.ToLower(st[:1]) + st[1:],
			Events:         b.Events,
			CapState:       st,
			Transitions:    stateTrans[state],
			TransitionsMap: tm,
			CommonsHere:    needCommons,
		}
		needCommons = false
		if err := exec(filepath.Join(cfg.Path, lowState+".go"), stateTempl, s); err != nil {
			return err
		}
		if !cfg.NoTests {
			if err := exec(filepath.Join(cfg.Path, lowState+"_test.go"), stateTestTempl, s); err != nil {
				return err
			}
		}
	}
	if err := exec(filepath.Join(cfg.Path, "events.go"), eventsTempl, b); err != nil {
		return err
	}
	if err := exec(filepath.Join(cfg.Path, "states.go"), statesTempl, b); err != nil {
		return err
	}
	if err := exec(filepath.Join(cfg.Path, "base.go"), baseTempl, b); err != nil {
		return err
	}
	return nil
}

func reserved(state string) bool {
	reserved := []string{
		"base",
		"states",
		"events",
	}
	for _, r := range reserved {
		if r == state {
			return true
		}
	}
	return false
}

func ensurePath(path string) error {
	return os.MkdirAll(path, 0755)
}

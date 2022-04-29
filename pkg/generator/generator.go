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
}

const eventsTemplate = `package {{.Package}}
{{- range .Events }}

// Event{{.Event}} is a constant for "{{.Event}}" event
const Event{{.Event}} = "{{.Event}}"
{{- end }}
`

const baseTemplate = `package {{.Package}}

import (
	"context"
	"github.com/fsmgo/fsmgo/pkg/fsm"
)

type Event struct {
	Type string
}
func (e Event) String() string {
	return e.Type
}

type StateContext struct {
}

type FSM = fsm.StateMachine[Event, StateContext]
type State = fsm.State[Event, StateContext]

type EventHandler interface {
	{{- range .Events }}
	// {{.Method}} processes event "{{.Event}}" 
	{{.Method}}(ctx context.Context, sc *StateContext) (State, error)
	{{- end }}
}
type base struct {
	h EventHandler
}
// OnError is a default error handler
func (s *base) OnError(ctx context.Context, d *FSM, err error) (State, error) {
	return nil, nil
}
// OnEnter is a default enter handler
func (s *base) OnEnter(ctx context.Context, d *FSM) error {
	return nil
}
// OnExit is a default exit handler
func (s *base) OnExit(ctx context.Context, d *FSM) error {
	return nil
}

// OnEvent is a main event handler function
func (s *base) OnEvent(ctx context.Context, d *FSM, e Event) (State, error) {
	switch e.Type {
	{{- range .Events }}
	case Event{{.Event}}:
		return s.h.{{.Method}}(ctx, d.StateContext)
	{{- end }}
	default:
		d.Logger.Warn().Str("event", e.Type).Msg("unknown event")
	}
	return nil, nil
}

{{- range .Events }}
// {{.Method}} is a default (dummy) handler of "{{.Event}}" event
func (s *base) {{.Method}}(ctx context.Context, sc *StateContext) (State, error) {
	return nil, nil
}
{{- end }}
`

const stateTemplate = `package {{.Package}}
{{- if .Transitions }}
import "context"
{{- end }}

type {{.State}}State struct {
	base
}
{{ $state := (print .State "State") }}
// {{.CapState}} is a singleton object for "{{.State}}"
var {{.CapState}} State = init{{.CapState}}()

func init{{.CapState}}() *{{ $state }}{
	s := &{{ $state }}{}
	s.h = s
	return s
}

func (s *{{$state}}) String() string {
	return "{{.CapState}}"
}
{{range .Transitions}}
func (s *{{$state}}) On{{.Event}}(ctx context.Context, sc *StateContext) (State, error) {
	return {{.ToState}}, nil
}
{{end}}
`

const stateTestTemplate = `package {{.Package}}

import (
	"context"
	"testing"

	"github.com/fsmgo/fsmgo/pkg/fsm"
	"github.com/rs/zerolog"
)

{{- if .CommonsHere }}

func newTestStateContext() *StateContext {
	return &StateContext{}
}

{{- end }}
{{- $st := .CapState }}
{{- $trs := .TransitionsMap }}
{{- range .Events }}
func Test{{$st}}{{.Method}}Async(t *testing.T) {
	cfg := fsm.Config{
		Id:     "test",
		Logger: zerolog.New(zerolog.NewTestWriter(t)),
	}
	stCtx := newTestStateContext()
	sm := fsm.NewStateMachine(&cfg, {{$st}}, stCtx)
	err := sm.AddEvent(Event{
		Type: "{{.Event}}",
	})
	if err != nil {
		t.Errorf("FSM error: %v", err)
	}
	sm.Stop()

	{{- if index $trs .Event }}
	expected := {{ index $trs .Event }}
	{{- else }}
	expected := {{ $st }}
	{{- end }}
	if expected.String() != sm.State() {
		t.Errorf("Unexpected state: got %v, expected %v\n", sm.State(), expected.String())
	}
}

func Test{{$st}}{{.Method}}Sync(t *testing.T) {
	cfg := fsm.Config{
		Id:     "test",
		Logger: zerolog.New(zerolog.NewTestWriter(t)),
	}
	sm := fsm.NewStateMachine(&cfg, {{$st}}, &StateContext{})
	ctx := context.Background()
	err := sm.ProcessEvent(ctx, Event{
		Type: "{{.Event}}",
	})
	if err != nil {
		t.Errorf("FSM error: %v", err)
	}

	{{- if index $trs .Event }}
	expected := {{ index $trs .Event }}
	{{- else }}
	expected := {{ $st }}
	{{- end }}
	if expected.String() != sm.State() {
		t.Errorf("Unexpected state: got %v, expected %v\n", sm.State(), expected.String())
	}
}
{{ end }}
`

type EventInfo struct {
	Event  string
	Method string
}

func convertCase(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.Title(s)
	return strings.ReplaceAll(s, " ", "")
}

func eventsToMethods(evs []string) []EventInfo {
	var rt []EventInfo
	for _, e := range evs {
		s := "On" + convertCase(e)
		rt = append(rt, EventInfo{e, s})
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
	if err := exec(filepath.Join(cfg.Path, "events.go"), eventsTempl, b); err != nil {
		return err
	}
	if err := exec(filepath.Join(cfg.Path, "base.go"), baseTempl, b); err != nil {
		return err
	}
	needCommons := true
	for _, state := range states {
		st := convertCase(state)
		tm := make(map[string]string)
		for _, v := range stateTrans[state] {
			tm[v.OrigEvent] = v.ToState
		}
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
		if err := exec(filepath.Join(cfg.Path, state+".go"), stateTempl, s); err != nil {
			return err
		}
		if !cfg.NoTests {
			if err := exec(filepath.Join(cfg.Path, state+"_test.go"), stateTestTempl, s); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensurePath(path string) error {
	return os.MkdirAll(path, 0755)
}

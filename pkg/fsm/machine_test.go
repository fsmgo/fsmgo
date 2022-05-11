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

package fsm

import (
	"context"
	"fmt"
	"testing"
)

type myEvent string

func (e myEvent) String() string {
	return string(e)
}

type myStateContext struct {
	x        int
	counters map[string]int
}
type errTransition struct {
	err   error
	state SMState
}

type SMState = State[myEvent, myStateContext]
type SM = StateMachine[myEvent, myStateContext]

func newStateMachine(cfg *Config, init SMState, data *myStateContext) (*SM, error) {
	return NewStateMachine[myEvent, myStateContext](cfg, init, data)
}

type errState struct {
}

func (s *errState) OnEnter(ctx context.Context, sm *SM, e myEvent) (SMState, error) {
	return nil, fmt.Errorf("err-OnEnter")
}
func (s *errState) OnExit(ctx context.Context, sm *SM, e myEvent) {
}
func (s *errState) OnError(ctx context.Context, sm *SM, e myEvent, err error) SMState {
	return nil
}
func (s *errState) String() string {
	return "err-state"
}
func (s *errState) OnEvent(ctx context.Context, d *SM, e myEvent) (SMState, error) {
	return nil, nil
}

//
type aState struct {
	name            string
	enterCalled     bool
	exitCalled      bool
	err             error
	eventsProcessed int
	transitions     map[myEvent]SMState
	errTransitions  map[myEvent]errTransition
}

func newState(name string) *aState {
	return &aState{
		name:           name,
		transitions:    make(map[myEvent]SMState),
		errTransitions: make(map[myEvent]errTransition),
	}
}

func (s *aState) OnEnter(ctx context.Context, sm *SM, e myEvent) (SMState, error) {
	s.enterCalled = true
	return nil, nil
}
func (s *aState) OnExit(ctx context.Context, sm *SM, e myEvent) {
	s.exitCalled = true
}
func (s *aState) OnError(ctx context.Context, sm *SM, e myEvent, err error) SMState {
	s.err = err
	return nil
}
func (s *aState) String() string {
	return s.name
}

func (s *aState) OnEvent(ctx context.Context,
	d *SM,
	e myEvent) (SMState, error) {
	s.eventsProcessed++
	if next, ok := s.errTransitions[e]; ok {
		if next.state == nil {
			return nil, next.err
		}
		return next.state, next.err
	}
	if next, ok := s.transitions[e]; ok {
		return next, nil
	}
	return nil, nil
}

func testFsmConfig() *Config {
	return &Config{
		Id:               "test-fsm",
		EventBacklogSize: 2,
	}
}

///

func TestCreateFSM(t *testing.T) {
	data := &myStateContext{
		x: 155,
	}
	state := newState("")
	cfg := &Config{
		Id:               "test-fsm",
		EventBacklogSize: 2,
	}
	_, _ = NewStateMachine[myEvent, myStateContext](cfg, state, data)
	if !state.enterCalled {
		t.Errorf("OnEnter not called on init state")
	}
	if state.exitCalled {
		t.Errorf("OnExit called on init state")
	}
	if state.eventsProcessed > 0 {
		t.Errorf("OnEvent called on init state")
	}
}

func TestTransition(t *testing.T) {
	data := &myStateContext{}
	state1 := newState("init")
	state2 := newState("next")

	state1.transitions["foo"] = state2

	sm, err := newStateMachine(testFsmConfig(), state1, data)
	err = sm.ProcessEvent(context.Background(), "foo")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if sm.State() != state2.String() {
		t.Errorf("Unexpected state: expected %s, got %s", state2.String(), sm.State())
	}
	if !state1.exitCalled {
		t.Errorf("OnExit not called on init state")
	}
	if !state2.enterCalled {
		t.Errorf("OnEnter not called on init state")
	}
	if state1.eventsProcessed != 1 {
		t.Errorf("Wrong events counter on init state: expected %d, got %d", 1, state1.eventsProcessed)
	}
}

func TestTransitionAsync(t *testing.T) {
	data := &myStateContext{
		x: 155,
	}
	state1 := newState("init")
	state2 := newState("next")

	state1.transitions["foo"] = state2

	sm, err := newStateMachine(testFsmConfig(), state1, data)
	err = sm.AddEvent("foo")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	sm.Stop()
	if sm.State() != state2.String() {
		t.Errorf("Unexpected state: expected %s, got %s", state2.String(), sm.State())
	}
	if !state1.exitCalled {
		t.Errorf("OnExit not called on init state")
	}
	if !state2.enterCalled {
		t.Errorf("OnEnter not called on init state")
	}
	if state1.eventsProcessed != 1 {
		t.Errorf("Wrong events counter on init state: expected %d, got %d", 1, state1.eventsProcessed)
	}
}

func TestTransitionUnknownEvent(t *testing.T) {
	data := &myStateContext{
		x: 155,
	}
	state1 := newState("init")
	state2 := newState("next")

	state1.transitions["foo"] = state2

	sm, err := newStateMachine(testFsmConfig(), state1, data)
	err = sm.ProcessEvent(context.Background(), "bar")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if sm.State() != state1.String() {
		t.Errorf("Unexpected state: expected %s, got %s", state2.String(), sm.State())
	}
	if state1.exitCalled {
		t.Errorf("OnExit called on init state")
	}
	if state2.enterCalled {
		t.Errorf("OnEnter called on next state")
	}
	if state1.eventsProcessed != 1 {
		t.Errorf("Wrong events counter on init state: expected %d, got %d", 1, state1.eventsProcessed)
	}
	if state1.err != nil {
		t.Errorf("OnError called on init state")
	}
}

func TestTransitionError(t *testing.T) {
	data := &myStateContext{
		x: 155,
	}
	state1 := newState("init")
	state2 := newState("next")
	someErr := fmt.Errorf("some error")
	state1.errTransitions["foo"] = errTransition{
		err:   someErr,
		state: state2,
	}

	sm, err := newStateMachine(testFsmConfig(), state1, data)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	err = sm.ProcessEvent(context.Background(), "foo")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if sm.State() != state2.String() {
		t.Errorf("Unexpected state: expected %s, got %s", state2.String(), sm.State())
	}
	if !state1.exitCalled {
		t.Errorf("OnExit not called on init state")
	}
	if !state2.enterCalled {
		t.Errorf("OnEnter not called on next state")
	}
	if state1.eventsProcessed != 1 {
		t.Errorf("Wrong events counter on init state: expected %d, got %d", 1, state1.eventsProcessed)
	}
	if state1.err != someErr {
		t.Errorf("OnError not called on init state")
	}
}

func TestTransitionError2(t *testing.T) {
	data := &myStateContext{
		x: 155,
	}
	state1 := newState("init")
	state2 := newState("next")
	someErr := fmt.Errorf("some error")
	state1.errTransitions["foo"] = errTransition{
		err: someErr,
	}

	sm, err := newStateMachine(testFsmConfig(), state1, data)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	err = sm.ProcessEvent(context.Background(), "foo")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if sm.State() != state1.String() {
		t.Errorf("Unexpected state: expected %s, got %s", state1.String(), sm.State())
	}
	if state1.exitCalled {
		t.Errorf("OnExit called on init state")
	}
	if state2.enterCalled {
		t.Errorf("OnEnter called on next state")
	}
	if state1.eventsProcessed != 1 {
		t.Errorf("Wrong events counter on init state: expected %d, got %d", 1, state1.eventsProcessed)
	}
	if state1.err != someErr {
		t.Errorf("OnError not called on init state")
	}
}

func TestTransitionEnterError(t *testing.T) {
	data := &myStateContext{
		x: 155,
	}
	state1 := newState("init")
	state2 := &errState{}
	state1.transitions["foo"] = state2

	sm, err := newStateMachine(testFsmConfig(), state1, data)
	err = sm.ProcessEvent(context.Background(), "foo")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if sm.State() != state2.String() {
		t.Errorf("Unexpected state: expected %s, got %s", state1.String(), sm.State())
	}
	if !state1.exitCalled {
		t.Errorf("OnExit not called on init state")
	}
	if state1.eventsProcessed != 1 {
		t.Errorf("Wrong events counter on init state: expected %d, got %d", 1, state1.eventsProcessed)
	}
}

func TestStopFSM(t *testing.T) {
	data := &myStateContext{}
	state1 := newState("init")

	sm, err := newStateMachine(testFsmConfig(), state1, data)
	sm.Stop()
	err = sm.AddEvent("foo")
	if err != ErrFSMStopped {
		t.Errorf("Error expected: expected: %v, got: %v", ErrFSMStopped, err)
	}
}
func TestStopFSM2(t *testing.T) {
	data := &myStateContext{}
	state1 := newState("init")

	sm, err := newStateMachine(testFsmConfig(), state1, data)
	sm.Stop()
	err = sm.ProcessEvent(context.TODO(), "foo")
	if err != ErrFSMStopped {
		t.Errorf("Error expected: expected: %v, got: %v", ErrFSMStopped, err)
	}
}

func TestDoubleStop(t *testing.T) {
	data := &myStateContext{}
	state1 := newState("init")

	sm, _ := newStateMachine(testFsmConfig(), state1, data)
	sm.Stop()
	sm.Stop()
}

func TestSkipInitEnter(t *testing.T) {
	data := &myStateContext{}
	state1 := newState("init")

	cfg := testFsmConfig()
	cfg.SkipInitEnter = true
	_, _ = newStateMachine(cfg, state1, data)
	if state1.enterCalled {
		t.Errorf("OnEnter called when cfg.SkipInitEnter flag is set")
	}
}

func TestSkipInitEnter2(t *testing.T) {
	data := &myStateContext{}
	state1 := newState("init")

	cfg := testFsmConfig()
	cfg.SkipInitEnter = false
	_, _ = newStateMachine(cfg, state1, data)
	if !state1.enterCalled {
		t.Errorf("OnEnter NOT called when cfg.SkipInitEnter flag is not set")
	}
}

type errEnterState struct {
	aState
	entCount int
}

func (s *errEnterState) OnEnter(ctx context.Context, sm *SM, e myEvent) (SMState, error) {
	sm.StateContext.counters["enter"]++
	return &errEnterState{entCount: s.entCount + 1}, fmt.Errorf("err-OnEnter")
}

func (s *errEnterState) OnError(ctx context.Context, sm *SM, e myEvent, err error) SMState {
	sm.StateContext.counters["errors"]++
	return &errEnterState{entCount: s.entCount + 1}
}

func TestOnEnterErr(t *testing.T) {
	data := &myStateContext{
		counters: make(map[string]int),
	}
	state1 := &errEnterState{}

	cfg := testFsmConfig()
	cfg.ErrLimit = 12

	_, err := newStateMachine(cfg, state1, data)
	if err != ErrFaultLimit {
		t.Errorf("a ErrFaultLimit error is expected")
	}
	if v := data.counters["enter"]; v != 12 {
		t.Errorf("12 enters expected, got %v", v)
	}
	if v := data.counters["errors"]; v != 12 {
		t.Errorf("12 errors expected, got %v", v)
	}
}

func TestTransitionSyncSM(t *testing.T) {
	data := &myStateContext{
		x: 155,
	}
	state1 := newState("init")
	state2 := newState("next")

	state1.transitions["foo"] = state2

	cfg := testFsmConfig()
	cfg.Sync = true

	sm, err := newStateMachine(cfg, state1, data)
	err = sm.AddEvent("foo")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	sm.Stop()
	if sm.State() != state2.String() {
		t.Errorf("Unexpected state: expected %s, got %s", state2.String(), sm.State())
	}
	if !state1.exitCalled {
		t.Errorf("OnExit not called on init state")
	}
	if !state2.enterCalled {
		t.Errorf("OnEnter not called on init state")
	}
	if state1.eventsProcessed != 1 {
		t.Errorf("Wrong events counter on init state: expected %d, got %d", 1, state1.eventsProcessed)
	}
}

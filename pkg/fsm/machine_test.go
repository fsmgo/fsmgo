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
	"github.com/rs/zerolog"
	"testing"
)

type myEvent string

func (e myEvent) String() string {
	return string(e)
}

type myStateContext struct {
	x int
}
type errTransition struct {
	err   error
	state State[myEvent, myStateContext]
}

type errState struct {
}

func (s *errState) OnEnter(ctx context.Context, sm *StateMachine[myEvent, myStateContext]) error {
	return fmt.Errorf("err-OnEnter")
}
func (s *errState) OnExit(ctx context.Context, sm *StateMachine[myEvent, myStateContext]) error {
	return fmt.Errorf("err-OnExit")

}
func (s *errState) OnError(ctx context.Context, sm *StateMachine[myEvent, myStateContext], err error) (State[myEvent, myStateContext], error) {
	return nil, fmt.Errorf("err-OnError")
}
func (s *errState) String() string {
	return "err-state"
}

func (s *errState) OnEvent(ctx context.Context,
	d *StateMachine[myEvent, myStateContext],
	e myEvent) (State[myEvent, myStateContext], error) {
	return nil, nil
}

//
type aState struct {
	name            string
	enterCalled     bool
	exitCalled      bool
	err             error
	eventsProcessed int
	transitions     map[myEvent]State[myEvent, myStateContext]
	errTransitions  map[myEvent]errTransition
}

func newState(name string) *aState {
	return &aState{
		name:           name,
		transitions:    make(map[myEvent]State[myEvent, myStateContext]),
		errTransitions: make(map[myEvent]errTransition),
	}
}

func (s *aState) OnEnter(ctx context.Context, sm *StateMachine[myEvent, myStateContext]) error {
	s.enterCalled = true
	return nil
}
func (s *aState) OnExit(ctx context.Context, sm *StateMachine[myEvent, myStateContext]) error {
	s.exitCalled = true
	return nil
}
func (s *aState) OnError(ctx context.Context, sm *StateMachine[myEvent, myStateContext], err error) (State[myEvent, myStateContext], error) {
	s.err = err
	return nil, nil
}
func (s *aState) String() string {
	return s.name
}

func (s *aState) OnEvent(ctx context.Context,
	d *StateMachine[myEvent, myStateContext],
	e myEvent) (State[myEvent, myStateContext], error) {
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
		Logger:           zerolog.New(zerolog.NewConsoleWriter()),
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
		Logger:           zerolog.New(zerolog.NewConsoleWriter()),
	}
	_ = NewStateMachine[myEvent, myStateContext](cfg, state, data)
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

	sm := NewStateMachine[myEvent, myStateContext](testFsmConfig(), state1, data)
	err := sm.ProcessEvent(context.Background(), "foo")
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

	sm := NewStateMachine[myEvent, myStateContext](testFsmConfig(), state1, data)
	err := sm.AddEvent("foo")
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

	sm := NewStateMachine[myEvent, myStateContext](testFsmConfig(), state1, data)
	err := sm.ProcessEvent(context.Background(), "bar")
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

	sm := NewStateMachine[myEvent, myStateContext](testFsmConfig(), state1, data)
	err := sm.ProcessEvent(context.Background(), "foo")
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

	sm := NewStateMachine[myEvent, myStateContext](testFsmConfig(), state1, data)
	err := sm.ProcessEvent(context.Background(), "foo")
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

	sm := NewStateMachine[myEvent, myStateContext](testFsmConfig(), state1, data)
	err := sm.ProcessEvent(context.Background(), "foo")
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

	sm := NewStateMachine[myEvent, myStateContext](testFsmConfig(), state1, data)
	sm.Stop()
	err := sm.AddEvent("foo")
	if err != ErrFSMStopped {
		t.Errorf("Error expected: expected: %v, got: %v", ErrFSMStopped, err)
	}
}
func TestStopFSM2(t *testing.T) {
	data := &myStateContext{}
	state1 := newState("init")

	sm := NewStateMachine[myEvent, myStateContext](testFsmConfig(), state1, data)
	sm.Stop()
	err := sm.ProcessEvent(context.TODO(), "foo")
	if err != ErrFSMStopped {
		t.Errorf("Error expected: expected: %v, got: %v", ErrFSMStopped, err)
	}
}

func TestDoubleStop(t *testing.T) {
	data := &myStateContext{}
	state1 := newState("init")

	sm := NewStateMachine[myEvent, myStateContext](testFsmConfig(), state1, data)
	sm.Stop()
	sm.Stop()
}

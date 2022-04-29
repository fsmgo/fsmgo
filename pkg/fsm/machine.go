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
	"errors"
	"fmt"
	"sync"

	"github.com/rs/zerolog"
)

// ErrFSMStopped is returned if event is added after the FSM has stopped
var ErrFSMStopped = errors.New("FSM is stopped")

// State is an interface for any state controlled by FSM
type State[E Event, D any] interface {
	OnEvent(ctx context.Context, sm *StateMachine[E, D], e E) (State[E, D], error)

	OnEnter(ctx context.Context, sm *StateMachine[E, D]) error
	OnExit(ctx context.Context, sm *StateMachine[E, D]) error
	OnError(ctx context.Context, sm *StateMachine[E, D], err error) (State[E, D], error)

	String() string
}

// Event is an interface for any event used by FSM
type Event interface {
	String() string
}

// Config is a configuration of a State machine
type Config struct {
	// Id is and id of the sm
	Id string
	// EventBacklogSize is the size of the Event queue
	EventBacklogSize uint
	// EventBacklogSize if is set, do not call OnEnter on initial state
	// when FSM created
	SkipInitEnter bool
	// Logger is the logger to Logger internal events
	Logger zerolog.Logger
}

// StateMachine is an implementation of FSM
type StateMachine[E Event, D any] struct {
	StateContext *D
	Logger       zerolog.Logger

	id string

	state  State[E, D]
	events chan E

	lock sync.Mutex
	evQ  sync.WaitGroup

	doneOnce sync.Once
	stopped  bool
	done     chan any
}

func (sm *StateMachine[E, D]) ProcessEvent(ctx context.Context, e E) error {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	if sm.stopped {
		return ErrFSMStopped
	}
	sm.evQ.Wait()

	sm.evQ.Add(1)
	sm.handleEvent(ctx, e)
	return nil
}

func (sm *StateMachine[E, D]) AddEvent(e E) error {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	if sm.stopped {
		return ErrFSMStopped
	}
	sm.evQ.Add(1)
	sm.events <- e
	return nil
}

func (sm *StateMachine[E, D]) Stop() {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	if sm.stopped {
		return
	}
	sm.evQ.Wait()

	sm.doneOnce.Do(func() {
		close(sm.done)
		sm.stopped = true
	})
}
func (sm *StateMachine[E, D]) State() string {
	return sm.state.String()
}

func (sm *StateMachine[E, D]) Done() <-chan any {
	return sm.done
}

func (sm *StateMachine[E, D]) handleEvent(ctx context.Context, ev E) {
	defer sm.evQ.Done()
	errLog := func(state fmt.Stringer, event fmt.Stringer, err error) *zerolog.Event {
		return sm.Logger.Error().Err(err).Stringer("State", state).Stringer("Event", event)
	}

	newSt, err := sm.state.OnEvent(ctx, sm, ev)
	if err != nil {
		errLog(sm.state, ev, err).Msg("failed to process event")
		errSt, err := sm.state.OnError(ctx, sm, err)
		if err != nil {
			errLog(sm.state, ev, err).Msg("failed to process error")
		}
		if errSt != nil {
			newSt = errSt
		}
	}
	if newSt == nil {
		newSt = sm.state
	} else {
		if err = sm.state.OnExit(ctx, sm); err != nil {
			errLog(sm.state, ev, err).Msg("failed to process onExit")
		}
		if err := newSt.OnEnter(ctx, sm); err != nil {
			errLog(newSt, ev, err).Msg("failed to process onEnter")
		}
	}
	sm.Logger.Debug().Msgf("{%s} transition [%s] (%s) => [%s]", sm.id, sm.state, ev, newSt.String())
	sm.state = newSt
}

func NewStateMachine[E Event, D any](cfg *Config, init State[E, D], data *D) *StateMachine[E, D] {
	rt := &StateMachine[E, D]{
		id:           cfg.Id,
		Logger:       cfg.Logger,
		StateContext: data,
		done:         make(chan any),
		events:       make(chan E, cfg.EventBacklogSize),
		state:        init,
	}
	if !cfg.SkipInitEnter {
		if err := rt.state.OnEnter(context.Background(), rt); err != nil {
			rt.state, _ = rt.state.OnError(context.Background(), rt, err)
		}
	}
	go func() {
		defer close(rt.events)
		ctx := context.Background()
		for {
			select {
			case <-rt.done:
				return
			case ev := <-rt.events:
				rt.handleEvent(ctx, ev)
			}
		}
	}()
	return rt
}

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
	"github.com/rs/zerolog/log"
	"sync"

	"github.com/rs/zerolog"
)

// ErrFSMStopped is returned if event is added after the FSM has stopped
var ErrFSMStopped = errors.New("FSM is stopped")

// ErrFaultLimit indicates the limit for OnError(..) errors was reached
var ErrFaultLimit = errors.New("fault limit reached")

// State is an interface for any state controlled by FSM
type State[E Event, D any] interface {
	OnEvent(ctx context.Context, sm *StateMachine[E, D], e E) (State[E, D], error)

	OnEnter(ctx context.Context, sm *StateMachine[E, D], e E) (State[E, D], error)
	OnExit(ctx context.Context, sm *StateMachine[E, D], e E)
	OnError(ctx context.Context, sm *StateMachine[E, D], e E, err error) State[E, D]

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
	// limit the number of OnEnter iterations
	ErrLimit int
	// if Sync, event queue is not used, event processing is always blocking
	Sync bool
	// Logger is used to log transitions for async events
	Logger zerolog.Logger
}

// StateMachine is an implementation of FSM
type StateMachine[E Event, D any] struct {
	StateContext *D
	Logger       zerolog.Logger

	id string

	state  State[E, D]
	events chan E

	lock     sync.Mutex
	evQ      sync.WaitGroup
	errLimit int
	doneOnce sync.Once
	stopped  bool
	async    bool
	done     chan any
}

func (sm *StateMachine[E, D]) ProcessEvent(ctx context.Context, e E) error {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	if sm.stopped {
		return ErrFSMStopped
	}
	if sm.async {
		sm.evQ.Wait()
		sm.evQ.Add(1)
	}
	sm.handleEvent(ctx, e)
	return nil
}

func (sm *StateMachine[E, D]) AddEvent(e E) error {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	if sm.stopped {
		return ErrFSMStopped
	}
	if sm.async {
		sm.evQ.Add(1)
		sm.events <- e
	} else {
		ctx := sm.Logger.WithContext(context.Background())
		sm.handleEvent(ctx, e)
	}
	return nil
}

func (sm *StateMachine[E, D]) Stop() {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	if sm.stopped {
		return
	}
	if sm.async {
		sm.evQ.Wait()
	}
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
	if sm.async {
		defer sm.evQ.Done()
	}
	l := log.Ctx(ctx).With().Stringer("State", sm.state).Stringer("Event", ev).Logger()

	newSt, err := sm.state.OnEvent(ctx, sm, ev)
	if err != nil {
		l.Error().Err(err).Msg("failed to process event")
		errSt := sm.state.OnError(ctx, sm, ev, err)
		if errSt != nil {
			newSt = errSt
		}
	}

	if newSt == nil {
		newSt = sm.state
	} else {
		sm.state.OnExit(ctx, sm, ev)
		st, err := sm.onEnterLoop(ctx, newSt, ev)
		if err != nil {
			l.Error().Err(err).Msg("failed to process onEnter")
		}
		if st != nil {
			newSt = st
		}
	}
	l.Debug().Msgf("{%s} transition [%s] (%s) => [%s]", sm.id, sm.state, ev, newSt.String())
	sm.state = newSt
}

func (sm *StateMachine[E, D]) onEnterLoop(ctx context.Context, s State[E, D], e E) (State[E, D], error) {
	limit := sm.errLimit
	shouldBreak := limit > 0
	for !shouldBreak || limit > 0 {
		next, err := s.OnEnter(ctx, sm, e)
		if err == nil {
			return next, nil
		}
		s = s.OnError(ctx, sm, e, err)
		if s == nil {
			return next, err
		}
		if shouldBreak {
			limit--
		}
	}
	return nil, ErrFaultLimit
}

func (sm *StateMachine[E, D]) onEnter(ctx context.Context) error {
	var e E
	st, err := sm.onEnterLoop(ctx, sm.state, e)
	if err != nil {
		return err
	}
	if st != nil {
		sm.state = st
	}
	return nil
}

func NewStateMachine[E Event, D any](cfg *Config, init State[E, D], data *D) (*StateMachine[E, D], error) {
	rt := &StateMachine[E, D]{
		id:           cfg.Id,
		StateContext: data,
		done:         make(chan any),
		state:        init,
		async:        !cfg.Sync,
		errLimit:     cfg.ErrLimit,
	}
	if !cfg.SkipInitEnter {
		if err := rt.onEnter(context.Background()); err != nil {
			return nil, err
		}
	}
	if rt.async {
		rt.events = make(chan E, cfg.EventBacklogSize)
		go func() {
			defer close(rt.events)
			ctx := rt.Logger.WithContext(context.Background())
			for {
				select {
				case <-rt.done:
					return
				case ev := <-rt.events:
					rt.handleEvent(ctx, ev)
				}
			}
		}()
	}
	return rt, nil
}

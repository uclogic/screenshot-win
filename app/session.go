package app

import (
	"errors"
	"fmt"
	"sync"
)

// State describes the lifecycle of the one capture session an App may own.
type State uint8

const (
	StateIdle State = iota
	StateSelecting
	StateFrozen
	StateScrolling
	StateEditing
)

var ErrSessionActive = errors.New("a capture session is already active")

// App coordinates capture sessions. It is intentionally independent from the
// Windows UI so the tray host can safely call it from its message loop.
type App struct {
	mu         sync.Mutex
	state      State
	generation uint64
}

// Session is the exclusive capture session currently owned by an App.
type Session struct {
	app        *App
	generation uint64
	once       sync.Once
}

func New() *App { return &App{} }

// State returns the current capture lifecycle state.
func (application *App) State() State {
	application.mu.Lock()
	defer application.mu.Unlock()
	return application.state
}

// Begin starts an exclusive session in the requested initial state.
func (application *App) Begin(initial State) (*Session, error) {
	if initial == StateIdle {
		return nil, fmt.Errorf("initial session state must not be idle")
	}
	application.mu.Lock()
	defer application.mu.Unlock()
	if application.state != StateIdle {
		return nil, ErrSessionActive
	}
	application.generation++
	application.state = initial
	return &Session{app: application, generation: application.generation}, nil
}

// Transition advances a live session only when it is in the expected state.
func (session *Session) Transition(expected, next State) error {
	if session == nil || session.app == nil {
		return errors.New("capture session is nil")
	}
	if next == StateIdle {
		return errors.New("use Finish to return a session to idle")
	}
	application := session.app
	application.mu.Lock()
	defer application.mu.Unlock()
	if application.generation != session.generation || application.state == StateIdle {
		return errors.New("capture session is no longer active")
	}
	if application.state != expected {
		return fmt.Errorf("capture session state is %v, expected %v", application.state, expected)
	}
	application.state = next
	return nil
}

// Finish releases the session. It is safe to call more than once.
func (session *Session) Finish() {
	if session == nil || session.app == nil {
		return
	}
	session.once.Do(func() {
		application := session.app
		application.mu.Lock()
		defer application.mu.Unlock()
		if application.generation == session.generation {
			application.state = StateIdle
		}
	})
}

func (state State) String() string {
	switch state {
	case StateIdle:
		return "idle"
	case StateSelecting:
		return "selecting"
	case StateFrozen:
		return "frozen"
	case StateScrolling:
		return "scrolling"
	case StateEditing:
		return "editing"
	default:
		return fmt.Sprintf("state(%d)", state)
	}
}

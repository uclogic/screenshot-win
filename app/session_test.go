package app

import (
	"errors"
	"sync"
	"testing"
)

func TestSessionLifecycle(t *testing.T) {
	application := New()
	session, err := application.Begin(StateSelecting)
	if err != nil {
		t.Fatal(err)
	}
	if got := application.State(); got != StateSelecting {
		t.Fatalf("state = %v, want selecting", got)
	}
	if err := session.Transition(StateSelecting, StateFrozen); err != nil {
		t.Fatal(err)
	}
	if err := session.Transition(StateFrozen, StateScrolling); err != nil {
		t.Fatal(err)
	}
	session.Finish()
	session.Finish()
	if got := application.State(); got != StateIdle {
		t.Fatalf("state = %v, want idle", got)
	}
}

func TestAppRejectsConcurrentSessions(t *testing.T) {
	application := New()
	session, err := application.Begin(StateSelecting)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Finish()
	if _, err := application.Begin(StateScrolling); !errors.Is(err, ErrSessionActive) {
		t.Fatalf("Begin() error = %v, want %v", err, ErrSessionActive)
	}
}

func TestAppAllowsOnlyOneConcurrentBegin(t *testing.T) {
	application := New()
	const callers = 20
	var wait sync.WaitGroup
	wait.Add(callers)
	results := make(chan *Session, callers)
	for range callers {
		go func() {
			defer wait.Done()
			session, err := application.Begin(StateSelecting)
			if err == nil {
				results <- session
			}
		}()
	}
	wait.Wait()
	close(results)
	var sessions []*Session
	for session := range results {
		sessions = append(sessions, session)
	}
	if len(sessions) != 1 {
		t.Fatalf("successful sessions = %d, want 1", len(sessions))
	}
	sessions[0].Finish()
}

func TestSessionRejectsUnexpectedTransition(t *testing.T) {
	application := New()
	session, err := application.Begin(StateSelecting)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Finish()
	if err := session.Transition(StateFrozen, StateScrolling); err == nil {
		t.Fatal("unexpected transition succeeded")
	}
	if got := application.State(); got != StateSelecting {
		t.Fatalf("state = %v, want selecting", got)
	}
}

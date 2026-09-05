package main

import (
	"errors"
	"reflect"
	"testing"
)

type fakeHotkeys struct {
	keys    map[uintptr]configuredHotkey
	calls   int
	failAt  int
	blocked configuredHotkey
}

func newFakeRegistry() (*hotkeyRegistry, *fakeHotkeys) {
	fake := &fakeHotkeys{keys: make(map[uintptr]configuredHotkey)}
	registry := &hotkeyRegistry{
		register: func(id uintptr, key configuredHotkey) error {
			fake.calls++
			if fake.calls == fake.failAt || key == fake.blocked {
				return errors.New("key occupied")
			}
			for _, existing := range fake.keys {
				if existing == key {
					return errors.New("duplicate registration")
				}
			}
			fake.keys[id] = key
			return nil
		},
		unregister: func(id uintptr) { delete(fake.keys, id) },
	}
	return registry, fake
}

func TestHotkeysExchangeDisableAndClose(t *testing.T) {
	registry, fake := newFakeRegistry()
	a, _ := parseConfiguredHotkey("Ctrl+A")
	b, _ := parseConfiguredHotkey("F2")
	if err := registry.apply(map[configuredHotkey]hotkeyAction{a: hotkeyCapture, b: hotkeyPin}, nil); err != nil {
		t.Fatal(err)
	}
	calls := fake.calls
	if err := registry.apply(map[configuredHotkey]hotkeyAction{a: hotkeyPin, b: hotkeyCapture}, nil); err != nil {
		t.Fatal(err)
	}
	if fake.calls != calls || registry.label(hotkeyCapture) != "F2" || registry.label(hotkeyPin) != "Ctrl+A" {
		t.Fatal("exchange did not reuse registrations")
	}
	if err := registry.apply(map[configuredHotkey]hotkeyAction{b: hotkeyCapture}, nil); err != nil {
		t.Fatal(err)
	}
	if len(fake.keys) != 1 || registry.label(hotkeyPin) != "" {
		t.Fatal("disabled pin remains registered")
	}
	registry.close()
	registry.close()
	if len(fake.keys) != 0 || len(registry.active) != 0 {
		t.Fatal("registrations leaked")
	}
}

func TestHotkeysFailurePreservesOldBindings(t *testing.T) {
	for _, failure := range []string{"register", "save"} {
		t.Run(failure, func(t *testing.T) {
			registry, fake := newFakeRegistry()
			a, _ := parseConfiguredHotkey("Ctrl+A")
			b, _ := parseConfiguredHotkey("Alt+B")
			c, _ := parseConfiguredHotkey("F3")
			if err := registry.apply(map[configuredHotkey]hotkeyAction{a: hotkeyCapture}, nil); err != nil {
				t.Fatal(err)
			}
			before := map[uintptr]configuredHotkey{}
			for id, key := range fake.keys {
				before[id] = key
			}
			saved := false
			if failure == "register" {
				fake.failAt = fake.calls + 2
			}
			err := registry.apply(map[configuredHotkey]hotkeyAction{b: hotkeyCapture, c: hotkeyPin}, func() error { saved = true; return errors.New("disk full") })
			if err == nil {
				t.Fatal("failure was ignored")
			}
			if failure == "register" && saved {
				t.Fatal("saved before all registrations succeeded")
			}
			if !reflect.DeepEqual(fake.keys, before) || registry.label(hotkeyCapture) != "Ctrl+A" || registry.label(hotkeyPin) != "" {
				t.Fatalf("old keys changed or candidate leaked: %+v", fake.keys)
			}
		})
	}
}

func TestHotkeysRestoreKeepsAvailableAction(t *testing.T) {
	registry, fake := newFakeRegistry()
	a, _ := parseConfiguredHotkey("Ctrl+A")
	b, _ := parseConfiguredHotkey("F2")
	fake.blocked = a
	if err := registry.restore(map[configuredHotkey]hotkeyAction{a: hotkeyCapture, b: hotkeyPin}); err == nil {
		t.Fatal("conflict not reported")
	}
	if registry.label(hotkeyPin) != "F2" || len(fake.keys) != 1 {
		t.Fatal("available action was lost")
	}
	fake.blocked = configuredHotkey{}
	if err := registry.restore(map[configuredHotkey]hotkeyAction{a: hotkeyCapture, b: hotkeyPin}); err != nil {
		t.Fatal(err)
	}
	if len(fake.keys) != 2 {
		t.Fatal("failed key was not recovered")
	}
}

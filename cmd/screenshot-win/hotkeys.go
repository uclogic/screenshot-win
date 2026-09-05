package main

import (
	"errors"
	"fmt"
)

type hotkeyAction uint8

const (
	hotkeyCapture hotkeyAction = iota
	hotkeyPin
)

type hotkeyBinding struct {
	key    configuredHotkey
	action hotkeyAction
}

// hotkeyRegistry owns registrations by chord, so exchanging actions never
// requires releasing and reacquiring a key that this process already owns.
type hotkeyRegistry struct {
	active     map[uintptr]hotkeyBinding
	register   func(uintptr, configuredHotkey) error
	unregister func(uintptr)
}

func preferenceHotkeys(value preferences) map[configuredHotkey]hotkeyAction {
	result := make(map[configuredHotkey]hotkeyAction)
	if normalizeOptionalHotkey(value.General.Hotkey) != "" {
		capture, _ := parseConfiguredHotkey(value.General.Hotkey)
		result[capture] = hotkeyCapture
	}
	if value.General.PinHotkey != "" {
		pin, _ := parseConfiguredHotkey(value.General.PinHotkey)
		result[pin] = hotkeyPin
	}
	return result
}

func (registry *hotkeyRegistry) apply(desired map[configuredHotkey]hotkeyAction, save func() error) error {
	candidate := make(map[uintptr]hotkeyBinding)
	added := make(map[uintptr]hotkeyBinding)
	rollback := func() {
		for id := range added {
			registry.unregister(id)
		}
	}
	for key, action := range desired {
		var existing uintptr
		for id, binding := range registry.active {
			if binding.key == key {
				existing = id
				break
			}
		}
		if existing != 0 {
			candidate[existing] = hotkeyBinding{key, action}
			continue
		}
		id := uintptr(1)
		for {
			_, old := registry.active[id]
			_, new := candidate[id]
			if !old && !new {
				break
			}
			id++
		}
		if err := registry.register(id, key); err != nil {
			rollback()
			return fmt.Errorf("%s: %w", formatConfiguredHotkey(key), err)
		}
		candidate[id] = hotkeyBinding{key, action}
		added[id] = candidate[id]
	}
	if save != nil {
		if err := save(); err != nil {
			rollback()
			return err
		}
	}
	for id := range registry.active {
		if _, keep := candidate[id]; !keep {
			registry.unregister(id)
		}
	}
	registry.active = candidate
	return nil
}

func (registry *hotkeyRegistry) close() {
	for id := range registry.active {
		registry.unregister(id)
	}
	registry.active = nil
}

func (registry *hotkeyRegistry) label(action hotkeyAction) string {
	for _, binding := range registry.active {
		if binding.action == action {
			return formatConfiguredHotkey(binding.key)
		}
	}
	return ""
}

// restore acquires each action independently so a conflict cannot disable other
// shortcuts during startup or after releasing registrations for key recording.
func (registry *hotkeyRegistry) restore(desired map[configuredHotkey]hotkeyAction) error {
	var failures []error
	for _, action := range []hotkeyAction{hotkeyCapture, hotkeyPin} {
		for key, target := range desired {
			if target != action {
				continue
			}
			available := make(map[configuredHotkey]hotkeyAction)
			for _, binding := range registry.active {
				available[binding.key] = binding.action
			}
			available[key] = target
			if err := registry.apply(available, nil); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}

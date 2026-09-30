package selector

import (
	"fmt"
	"slices"
)

// ToolbarKind identifies the two independently configurable toolbars.
type ToolbarKind uint8

const (
	ScreenshotToolbar ToolbarKind = iota
	LongCaptureToolbar
)

// ToolbarItem is a stable setting ID and its corresponding command.
type ToolbarItem struct {
	ID     string
	Action Action
	Label  string
}

var toolbarItems = []ToolbarItem{
	{"rectangle", ActionRectangle, "Rectangle"}, {"arrow", ActionArrow, "Arrow"},
	{"text", ActionText, "Text"}, {"color", ActionColor, "Color"},
	{"width", ActionWidth, "Line width"}, {"scroll", ActionScroll, "Scrolling capture"},
	{"edit", ActionEdit, "Stop and annotate"}, {"pin", ActionPin, "Pin to desktop"},
	{"save", ActionSave, "Save"}, {"save_as", ActionSaveAs, "Save as"},
	{"copy", ActionCopy, "Copy"}, {"cancel", ActionCancel, "Cancel"},
}

func ToolbarItemForID(id string) (ToolbarItem, bool) {
	for _, item := range toolbarItems {
		if item.ID == id {
			return item, true
		}
	}
	return ToolbarItem{}, false
}

// DefaultToolbarLayout returns an independent copy of the factory layout.
func DefaultToolbarLayout(kind ToolbarKind) []string {
	actions := selectionToolbarActions
	if kind == LongCaptureToolbar {
		actions = captureToolbarActions
	}
	ids := make([]string, 0, len(actions))
	for _, action := range actions {
		for _, item := range toolbarItems {
			if item.Action == action {
				ids = append(ids, item.ID)
				break
			}
		}
	}
	return ids
}

// ResolveToolbarLayout treats nil as the default; an explicit empty layout is invalid.
func ResolveToolbarLayout(kind ToolbarKind, ids []string) ([]string, error) {
	if kind != ScreenshotToolbar && kind != LongCaptureToolbar {
		return nil, fmt.Errorf("unknown toolbar kind %d", kind)
	}
	if ids == nil {
		ids = DefaultToolbarLayout(kind)
	}
	allowed := DefaultToolbarLayout(kind)
	seen := make(map[string]bool)
	complete := false
	for _, id := range ids {
		if !slices.Contains(allowed, id) {
			return nil, fmt.Errorf("unsupported toolbar item %q", id)
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate toolbar item %q", id)
		}
		seen[id] = true
		if id == "pin" || id == "save" || id == "save_as" || id == "copy" || id == "edit" {
			complete = true
		}
	}
	if !complete {
		return nil, fmt.Errorf("toolbar must retain at least one completion action")
	}
	return slices.Clone(ids), nil
}

// ToolbarCandidates returns hidden items in factory order.
func ToolbarCandidates(kind ToolbarKind, ids []string) []string {
	result := []string{}
	for _, id := range DefaultToolbarLayout(kind) {
		if !slices.Contains(ids, id) {
			result = append(result, id)
		}
	}
	return result
}

// ChangeToolbarLayout inserts/moves an item, or removes it when index is negative.
// An invalid edit returns an error without changing the input.
func ChangeToolbarLayout(kind ToolbarKind, ids []string, id string, index int) ([]string, error) {
	if !slices.Contains(DefaultToolbarLayout(kind), id) {
		return nil, fmt.Errorf("unsupported toolbar item %q", id)
	}
	next := slices.Clone(ids)
	old := slices.Index(next, id)
	if old >= 0 {
		next = slices.Delete(next, old, old+1)
		if index > old {
			index--
		}
	}
	if index >= 0 {
		if index > len(next) {
			index = len(next)
		}
		next = slices.Insert(next, index, id)
	}
	return ResolveToolbarLayout(kind, next)
}

func toolbarLayoutActions(kind ToolbarKind, ids []string, annotation bool) ([]Action, []string, error) {
	resolved, err := ResolveToolbarLayout(kind, ids)
	if err != nil {
		return nil, nil, err
	}
	var actions []Action
	var labels []string
	for _, id := range resolved {
		item, _ := ToolbarItemForID(id)
		if annotation && item.Action == ActionScroll {
			continue
		}
		actions = append(actions, item.Action)
		labels = append(labels, item.Label)
	}
	return actions, labels, nil
}

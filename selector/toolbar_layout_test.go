package selector

import (
	"reflect"
	"testing"
)

func TestToolbarLayoutValidation(t *testing.T) {
	for _, kind := range []ToolbarKind{ScreenshotToolbar, LongCaptureToolbar} {
		defaults, err := ResolveToolbarLayout(kind, nil)
		if err != nil {
			t.Fatal(err)
		}
		defaults[0] = "modified"
		if DefaultToolbarLayout(kind)[0] == "modified" {
			t.Fatal("defaults shared storage")
		}
		for _, ids := range [][]string{{}, {"cancel"}, {"rectangle"}, {"copy", "copy"}, {"copy", "missing"}} {
			if _, err := ResolveToolbarLayout(kind, ids); err == nil {
				t.Fatalf("accepted kind=%v ids=%v", kind, ids)
			}
		}
		got, err := ResolveToolbarLayout(kind, []string{"copy"})
		if err != nil || !reflect.DeepEqual(got, []string{"copy"}) {
			t.Fatalf("cancel-free layout: %v %v", got, err)
		}
	}
	if _, err := ResolveToolbarLayout(ScreenshotToolbar, []string{"edit"}); err == nil {
		t.Fatal("accepted long-capture action in screenshot")
	}
	if _, err := ResolveToolbarLayout(LongCaptureToolbar, []string{"save"}); err == nil {
		t.Fatal("accepted screenshot action in long capture")
	}
	if _, err := ResolveToolbarLayout(ToolbarKind(99), nil); err == nil {
		t.Fatal("accepted unknown kind")
	}
}

func TestToolbarLayoutEditing(t *testing.T) {
	original := []string{"rectangle", "copy", "cancel"}
	tests := []struct {
		name, id string
		index    int
		want     []string
		invalid  bool
	}{
		{"move to end", "rectangle", 3, []string{"copy", "cancel", "rectangle"}, false},
		{"move to start", "cancel", 0, []string{"cancel", "rectangle", "copy"}, false},
		{"move after self", "copy", 2, original, false},
		{"add at gap", "arrow", 1, []string{"rectangle", "arrow", "copy", "cancel"}, false},
		{"add at end", "text", 3, []string{"rectangle", "copy", "cancel", "text"}, false},
		{"remove cancel", "cancel", -1, []string{"rectangle", "copy"}, false},
		{"remove completion", "copy", -1, nil, true},
		{"foreign action", "edit", 0, nil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := append([]string(nil), original...)
			got, err := ChangeToolbarLayout(ScreenshotToolbar, input, test.id, test.index)
			if (err != nil) != test.invalid || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %v %v, want %v invalid=%v", got, err, test.want, test.invalid)
			}
			if !reflect.DeepEqual(input, original) {
				t.Fatal("edit mutated input")
			}
		})
	}
	// Each long-capture finishing action can stand alone; removing it is rejected.
	for _, id := range []string{"edit", "pin", "save_as", "copy"} {
		if _, err := ChangeToolbarLayout(LongCaptureToolbar, []string{id}, id, -1); err == nil {
			t.Fatalf("removed final action %s", id)
		}
	}
}

func TestToolbarLayoutAnnotationAndCandidates(t *testing.T) {
	defaults, _, err := toolbarLayoutActions(ScreenshotToolbar, nil, true)
	if err != nil || !reflect.DeepEqual(defaults, annotationToolbarActions) {
		t.Fatalf("default annotation: %v %v", defaults, err)
	}
	ids := []string{"cancel", "scroll", "color", "copy", "arrow"}
	actions, labels, err := toolbarLayoutActions(ScreenshotToolbar, ids, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actions, []Action{ActionCancel, ActionColor, ActionCopy, ActionArrow}) {
		t.Fatalf("annotation actions %v", actions)
	}
	if !reflect.DeepEqual(labels, []string{"Cancel", "Color", "Copy", "Arrow"}) {
		t.Fatalf("annotation labels %v", labels)
	}
	candidates := ToolbarCandidates(ScreenshotToolbar, ids)
	if !reflect.DeepEqual(candidates, []string{"rectangle", "text", "width", "pin", "save"}) {
		t.Fatalf("candidates %v", candidates)
	}
	if len(ToolbarCandidates(LongCaptureToolbar, DefaultToolbarLayout(LongCaptureToolbar))) != 0 {
		t.Fatal("default has hidden candidates")
	}
}

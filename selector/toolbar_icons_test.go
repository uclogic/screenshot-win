package selector

import (
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolbarGlyphMappings(t *testing.T) {
	wantSources := map[Action]string{
		ActionCancel:    "x.svg",
		ActionSave:      "device-floppy.svg",
		ActionCopy:      "copy.svg",
		ActionScroll:    "square-rounded-arrow-down.svg",
		ActionSaveAs:    "file-download.svg",
		ActionPin:       "pin.svg",
		ActionEdit:      "edit.svg",
		ActionRectangle: "rectangle.svg",
		ActionArrow:     "arrow-up-right.svg",
		ActionText:      "letter-t.svg",
		ActionColor:     "palette.svg",
		ActionWidth:     "line.svg",
	}
	if len(toolbarGlyphs) != len(wantSources) {
		t.Fatalf("toolbar glyph count = %d, want %d", len(toolbarGlyphs), len(wantSources))
	}
	for action, source := range wantSources {
		glyph, ok := toolbarGlyphForAction(action)
		if !ok {
			t.Errorf("toolbar action %d has no glyph", action)
			continue
		}
		if glyph.source != source {
			t.Errorf("toolbar action %d source = %q, want %q", action, glyph.source, source)
		}
		if len(glyph.commands) == 0 {
			t.Errorf("toolbar action %d has an empty glyph", action)
		}
	}

	for _, actions := range [][]Action{selectionToolbarActions, annotationToolbarActions, captureToolbarActions} {
		for _, action := range actions {
			if _, ok := toolbarGlyphForAction(action); !ok {
				t.Errorf("toolbar action list contains unmapped action %d", action)
			}
		}
	}
}

func TestToolbarGlyphCoordinatesStayInViewBox(t *testing.T) {
	for action, glyph := range toolbarGlyphs {
		for commandIndex, command := range glyph.commands {
			coordinateCount := 0
			switch command.op {
			case toolbarGlyphMove, toolbarGlyphLine:
				coordinateCount = 2
			case toolbarGlyphCubic:
				coordinateCount = 6
			case toolbarGlyphClose:
				continue
			default:
				t.Fatalf("action %d command %d has unknown operation %d", action, commandIndex, command.op)
			}
			for coordinateIndex := 0; coordinateIndex < coordinateCount; coordinateIndex++ {
				value := command.args[coordinateIndex]
				if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) || value < 0 || value > toolbarGlyphViewBoxForTest {
					t.Errorf("action %d command %d coordinate %d = %v, want 0..24", action, commandIndex, coordinateIndex, value)
				}
			}
		}
	}
}

const toolbarGlyphViewBoxForTest = 24

func TestPinnedToolbarSVGAssets(t *testing.T) {
	root := filepath.Join("..", "third_party", "tabler-icons-v3.46.0")
	seen := make(map[string]bool)
	for _, glyph := range toolbarGlyphs {
		if seen[glyph.source] {
			continue
		}
		seen[glyph.source] = true
		contents, err := os.ReadFile(filepath.Join(root, glyph.source))
		if err != nil {
			t.Errorf("read pinned SVG %q: %v", glyph.source, err)
			continue
		}
		text := string(contents)
		if !strings.Contains(text, `viewBox="0 0 24 24"`) || !strings.Contains(text, `stroke-linecap="round"`) || !strings.Contains(text, `stroke-linejoin="round"`) {
			t.Errorf("pinned SVG %q does not retain the Tabler outline geometry metadata", glyph.source)
		}
	}
	license, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		t.Fatalf("read Tabler license: %v", err)
	}
	if !strings.Contains(string(license), "MIT License") || !strings.Contains(string(license), "Paweł Kuna") {
		t.Fatal("pinned Tabler license is incomplete")
	}
}

func TestToolbarIconStateValues(t *testing.T) {
	if got := toolbarIconBaseColor(true); got != (color.NRGBA{R: 235, G: 238, B: 242, A: 255}) {
		t.Fatalf("enabled base color = %+v", got)
	}
	if got := toolbarIconBaseColor(false); got != (color.NRGBA{R: 112, G: 116, B: 122, A: 255}) {
		t.Fatalf("disabled base color = %+v", got)
	}
	selected := color.NRGBA{R: 10, G: 20, B: 30, A: 4}
	if got := toolbarIconValueColor(selected, true); got != (color.NRGBA{R: 10, G: 20, B: 30, A: 255}) {
		t.Fatalf("enabled value color = %+v", got)
	}
	if got := toolbarIconValueColor(selected, false); got != toolbarIconBaseColor(false) {
		t.Fatalf("disabled value color = %+v", got)
	}
	if got := toolbarIconStrokeWidth(96); got != 2 {
		t.Fatalf("96 DPI icon stroke = %v, want 2", got)
	}
	if got := toolbarIconStrokeWidth(144); got != 3 {
		t.Fatalf("144 DPI icon stroke = %v, want 3", got)
	}
	if got := toolbarValueStrokeWidth(3, 144); got != 4.5 {
		t.Fatalf("144 DPI selected width = %v, want 4.5", got)
	}
	if got := toolbarValueStrokeWidth(0, 96); got != 1 {
		t.Fatalf("zero selected width = %v, want 1", got)
	}
}

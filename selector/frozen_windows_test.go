//go:build windows

package selector

import (
	"bytes"
	"fmt"
	"image"
	"testing"

	"screenshot-win/editor"
)

func TestDraftArrowTouchesOnlyVectorPixelsAndRestoresThem(t *testing.T) {
	const width, height = 1000, 700
	pixels := make([]byte, width*height*4)
	for index := range pixels {
		pixels[index] = byte(index % 251)
	}
	original := append([]byte(nil), pixels...)
	state := &frozenState{selectionState: &selectionState{client: image.Rect(0, 0, width, height), pixels: pixels}, region: image.Rect(0, 0, width, height)}
	seen := make(map[int]int)
	state.drawDraftLine(image.Pt(10, 10), image.Pt(980, 680), 3, editor.DefaultStyle(), seen)
	if len(state.draftPixels) == 0 {
		t.Fatal("draft arrow touched no pixels")
	}
	if len(state.draftPixels) >= width*height/20 {
		t.Fatalf("draft touched %d pixels; expected work proportional to vector, not canvas", len(state.draftPixels))
	}
	if bytes.Equal(state.pixels, original) {
		t.Fatal("draft did not change the surface")
	}
	state.restoreDraftPixels()
	if !bytes.Equal(state.pixels, original) {
		t.Fatal("restoring draft pixels did not recover the stable surface")
	}
}

func TestFrozenShortcutMapping(t *testing.T) {
	tests := []struct {
		key             uintptr
		control, shift  bool
		command, amount int
	}{
		{'Z', true, false, shortcutUndo, 0},
		{'Z', true, true, shortcutRedo, 0},
		{'Y', true, false, shortcutRedo, 0},
		{vkBack, false, false, shortcutDelete, 0},
		{vkDelete, false, false, shortcutDelete, 0},
		{vkLeft, false, false, shortcutLeft, 1},
		{vkDown, false, true, shortcutDown, 10},
		{vkEscape, false, false, shortcutEscape, 0},
	}
	for _, test := range tests {
		command, amount, ok := frozenShortcutForKey(test.key, test.control, test.shift)
		if !ok || command != test.command || amount != test.amount {
			t.Errorf("shortcut key=%d ctrl=%v shift=%v = (%d,%d,%v), want (%d,%d,true)", test.key, test.control, test.shift, command, amount, ok, test.command, test.amount)
		}
	}
	if _, _, ok := frozenShortcutForKey('A', false, false); ok {
		t.Fatal("plain A should not be handled as an editor shortcut")
	}
	if _, _, ok := frozenShortcutForKey(vkBack, true, false); ok {
		t.Fatal("Ctrl+Backspace should remain available to text input")
	}
}

func TestDeleteSelectedRemovesAnnotationAndClearsSelection(t *testing.T) {
	document, err := editor.NewDocument(image.NewRGBA(image.Rect(0, 0, 120, 80)))
	if err != nil {
		t.Fatal(err)
	}
	id, err := document.Add(editor.Annotation{Tool: editor.ToolRectangle, Start: image.Pt(10, 10), End: image.Pt(50, 40), Style: editor.DefaultStyle()})
	if err != nil {
		t.Fatal(err)
	}
	state := &frozenState{document: document, selected: id}
	if !state.deleteSelected() {
		t.Fatal("selected annotation was not deleted")
	}
	if state.selected != 0 {
		t.Fatalf("selection = %d after delete, want 0", state.selected)
	}
	if _, ok := document.Get(id); ok {
		t.Fatal("deleted annotation remains in the document")
	}
	if state.deleteSelected() {
		t.Fatal("deleting an empty selection reported a change")
	}
}

func TestFrozenSelectionHandlesAndZoomTolerance(t *testing.T) {
	state := &frozenState{viewport: editor.Viewport{Scale: 1, Offset: image.Pt(10, 15)}, dpi: 96}
	rectangle := editor.Annotation{Tool: editor.ToolRectangle, Start: image.Pt(20, 30), End: image.Pt(80, 70), Style: editor.DefaultStyle()}
	handles := state.handles(rectangle)
	if len(handles) != 8 {
		t.Fatalf("rectangle handles = %d, want 8", len(handles))
	}
	if handles[0].point != image.Pt(30, 45) || handles[4].point != image.Pt(90, 85) {
		t.Fatalf("corner handles = %v / %v", handles[0].point, handles[4].point)
	}
	state.viewport.Scale = .5
	zoomedOut := state.hitTolerance()
	state.viewport.Scale = 2
	zoomedIn := state.hitTolerance()
	if zoomedOut != zoomedIn*4 {
		t.Fatalf("hit tolerance out/in = %v/%v, want 4x", zoomedOut, zoomedIn)
	}
}

func TestClearSelectionOverlayRestoresDocumentPixels(t *testing.T) {
	const width, height = 140, 90
	source := image.NewRGBA(image.Rect(0, 0, width, height))
	document, err := editor.NewDocument(source)
	if err != nil {
		t.Fatal(err)
	}
	id, err := document.Add(editor.Annotation{
		Tool: editor.ToolRectangle, Start: image.Pt(25, 20), End: image.Pt(110, 65), Style: editor.DefaultStyle(),
	})
	if err != nil {
		t.Fatal(err)
	}
	annotation, _ := document.Get(id)
	pixels := make([]byte, width*height*4)
	state := &frozenState{
		selectionState: &selectionState{client: image.Rect(0, 0, width, height), pixels: pixels},
		region:         image.Rect(0, 0, width, height), document: document,
		viewport: editor.Viewport{Scale: 1}, dpi: 96,
	}
	state.copyViewport(editor.RenderViewport(document.Rendered(), state.viewport, state.region), state.region)
	drawOuterPixelBorder(state.pixels, state.client.Dx(), state.client, state.region)
	want := append([]byte(nil), state.pixels...)
	state.drawSelectionOverlay(annotation)
	if bytes.Equal(state.pixels, want) {
		t.Fatal("selection overlay did not change any pixels")
	}
	if err := state.clearSelectionOverlay(annotation); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(state.pixels, want) {
		t.Fatal("clearing selection overlay did not restore the stable document pixels")
	}
}

func TestDrawingGestureUsesDPIScaledThreshold(t *testing.T) {
	document, err := editor.NewDocument(image.NewRGBA(image.Rect(0, 0, 200, 120)))
	if err != nil {
		t.Fatal(err)
	}
	state := &frozenState{document: document, viewport: editor.Viewport{Scale: 1}, dpi: 96}
	request := &frozenAnnotationRequest{}
	state.beginDrawingGesture(request, image.Pt(20, 30))
	if state.updateDrawingGesture(request, image.Pt(23, 30)) {
		t.Fatal("three-pixel pointer jitter started a drawing gesture")
	}
	if !state.updateDrawingGesture(request, image.Pt(24, 30)) {
		t.Fatal("four-pixel movement did not start a drawing gesture at 96 DPI")
	}

	state.dpi = 192
	state.resetDrawingGesture(request)
	state.beginDrawingGesture(request, image.Pt(20, 30))
	if state.updateDrawingGesture(request, image.Pt(27, 30)) {
		t.Fatal("seven-pixel movement started a drawing gesture at 192 DPI")
	}
	if !state.updateDrawingGesture(request, image.Pt(28, 30)) {
		t.Fatal("eight-pixel movement did not start a drawing gesture at 192 DPI")
	}
}

func TestActiveToolClickSelectsTopmostAnnotationAndAdoptsItsStyle(t *testing.T) {
	document, err := editor.NewDocument(image.NewRGBA(image.Rect(0, 0, 120, 80)))
	if err != nil {
		t.Fatal(err)
	}
	bottomStyle := editor.DefaultStyle()
	bottomStyle.Color = editor.PresetColors()[0]
	if _, err := document.Add(editor.Annotation{Tool: editor.ToolArrow, Start: image.Pt(10, 30), End: image.Pt(100, 30), Style: bottomStyle}); err != nil {
		t.Fatal(err)
	}
	topStyle := editor.DefaultStyle()
	topStyle.Color = editor.PresetColors()[2]
	topID, err := document.Add(editor.Annotation{Tool: editor.ToolArrow, Start: image.Pt(20, 30), End: image.Pt(90, 30), Style: topStyle})
	if err != nil {
		t.Fatal(err)
	}
	styles := make(chan editor.Style, 1)
	request := &frozenAnnotationRequest{tool: editor.ToolRectangle, style: bottomStyle}
	state := &frozenState{
		selectionState: &selectionState{}, document: document, viewport: editor.Viewport{Scale: 1}, dpi: 96,
		request: request, styleEvents: styles,
	}
	if !state.selectAnnotationAt(image.Pt(50, 30)) {
		t.Fatal("click did not select an existing annotation")
	}
	if state.selected != topID {
		t.Fatalf("selected annotation = %d, want topmost %d", state.selected, topID)
	}
	if request.style != topStyle {
		t.Fatalf("active drawing style = %+v, want selected style %+v", request.style, topStyle)
	}
	if notified := <-styles; notified != topStyle {
		t.Fatalf("notified style = %+v, want %+v", notified, topStyle)
	}
	cleared, ok := state.clearSelectionForDrawing()
	if !ok || cleared.ID != topID || state.selected != 0 {
		t.Fatal("starting a new drawing did not clear the editing selection")
	}
	if _, ok := state.clearSelectionForDrawing(); ok {
		t.Fatal("clearing an already empty selection reported a change")
	}
}

func TestPersistentToolbarAcceptsOneActionUntilRearmed(t *testing.T) {
	events := make(chan ToolbarEvent, 1)
	state := &toolbarState{persistent: true, ready: true, events: events}
	if !state.emitPersistentAction(ActionArrow) {
		t.Fatal("ready toolbar did not emit its first action")
	}
	if state.ready {
		t.Fatal("toolbar remained ready while annotation action is active")
	}
	if !state.active || state.activeAction != ActionArrow {
		t.Fatalf("active tool = (%v, %v), want arrow", state.active, state.activeAction)
	}
	if state.emitPersistentAction(ActionRectangle) {
		t.Fatal("toolbar emitted a second action before being rearmed")
	}
	if got := <-events; got.Action != ActionArrow {
		t.Fatalf("emitted action = %v, want arrow", got)
	}

	state.ready = true
	state.active = false
	if !state.emitPersistentAction(ActionColor) {
		t.Fatal("rearmed toolbar did not emit style action")
	}
	if state.active {
		t.Fatal("style action should not remain highlighted as a drawing tool")
	}
}

func TestPersistentToolbarQueuesLatestDrawingToolSwitchUntilRearmed(t *testing.T) {
	events := make(chan ToolbarEvent, 1)
	state := &toolbarState{persistent: true, ready: true, events: events}
	if !state.handlePersistentAction(ActionText) {
		t.Fatal("ready toolbar did not emit text action")
	}
	if got := <-events; got.Action != ActionText {
		t.Fatalf("first action = %v, want text", got.Action)
	}
	if !state.handlePersistentAction(ActionRectangle) {
		t.Fatal("active text tool did not queue rectangle switch")
	}
	if !state.pending || state.pendingAction != ActionRectangle || state.activeAction != ActionRectangle {
		t.Fatalf("queued rectangle state = pending:%v pendingAction:%v activeAction:%v", state.pending, state.pendingAction, state.activeAction)
	}
	if !state.handlePersistentAction(ActionArrow) {
		t.Fatal("queued rectangle tool did not switch to arrow")
	}
	if state.handlePersistentAction(ActionArrow) {
		t.Fatal("clicking the already highlighted pending tool should be a no-op")
	}
	state.rearmPersistentActions()
	if state.pending || state.ready || !state.active || state.activeAction != ActionArrow {
		t.Fatalf("rearmed arrow state = pending:%v ready:%v active:%v action:%v", state.pending, state.ready, state.active, state.activeAction)
	}
	if got := <-events; got.Action != ActionArrow {
		t.Fatalf("switched action = %v, want arrow", got.Action)
	}
}

func TestPersistentToolbarQueuesOrdinaryActionWhileDrawingToolIsActive(t *testing.T) {
	for _, action := range []Action{ActionCancel, ActionSave, ActionCopy, ActionScroll, ActionPin} {
		t.Run(fmt.Sprint(action), func(t *testing.T) {
			events := make(chan ToolbarEvent, 1)
			state := &toolbarState{persistent: true, ready: true, events: events}
			if !state.handlePersistentAction(ActionRectangle) {
				t.Fatal("ready toolbar did not emit rectangle action")
			}
			if got := <-events; got.Action != ActionRectangle {
				t.Fatalf("first action = %v, want rectangle", got.Action)
			}
			if !state.handlePersistentAction(action) {
				t.Fatalf("active rectangle tool did not queue action %v", action)
			}
			if !state.pending || state.pendingAction != action || state.activeAction != action {
				t.Fatalf("queued action state = pending:%v pendingAction:%v activeAction:%v", state.pending, state.pendingAction, state.activeAction)
			}
			state.rearmPersistentActions()
			if state.pending || state.ready || state.active || state.activeAction != action {
				t.Fatalf("rearmed action state = pending:%v ready:%v active:%v action:%v", state.pending, state.ready, state.active, state.activeAction)
			}
			if got := <-events; got.Action != action {
				t.Fatalf("switched action = %v, want %v", got.Action, action)
			}
		})
	}
}

func TestPersistentToolbarReplacesQueuedActionWithoutRequiringAnotherToolExit(t *testing.T) {
	events := make(chan ToolbarEvent, 1)
	state := &toolbarState{persistent: true, ready: true, events: events}
	state.handlePersistentAction(ActionText)
	<-events
	state.handlePersistentAction(ActionSave)
	if !state.handlePersistentAction(ActionPin) {
		t.Fatal("queued save action could not be replaced with pin")
	}
	if !state.pending || state.pendingAction != ActionPin || state.activeAction != ActionPin {
		t.Fatalf("replaced pending state = pending:%v pendingAction:%v activeAction:%v", state.pending, state.pendingAction, state.activeAction)
	}
	state.rearmPersistentActions()
	if got := <-events; got.Action != ActionPin {
		t.Fatalf("replaced action = %v, want pin", got.Action)
	}
}

func TestToolSwitchFinishesUnplacedRequestButPreservesCapturedDrag(t *testing.T) {
	unplaced := &frozenAnnotationRequest{tool: editor.ToolText, result: make(chan error, 1)}
	state := &frozenState{selectionState: &selectionState{}, request: unplaced}
	state.finishCurrentToolForSwitch(editor.ToolText)
	if state.activeRequest() != nil {
		t.Fatal("unplaced request remained active after tool switch")
	}
	if err := <-unplaced.result; err != nil {
		t.Fatalf("unplaced request completed with error: %v", err)
	}

	dragging := &frozenAnnotationRequest{tool: editor.ToolArrow, dragging: true, result: make(chan error, 1)}
	state.request = dragging
	state.finishCurrentToolForSwitch(editor.ToolArrow)
	if state.activeRequest() != dragging {
		t.Fatal("captured drag was interrupted by tool switch")
	}
	if !state.toolSwapWait || state.toolSwapFrom != editor.ToolArrow {
		t.Fatal("tool switch requested during capture was not retained")
	}
}

func TestCompletedPlacementsStaySelectedAndKeepDrawingToolActive(t *testing.T) {
	arrow := &frozenAnnotationRequest{
		tool: editor.ToolArrow, result: make(chan error, 1),
		start: image.Pt(10, 20), current: image.Pt(30, 40),
	}
	state := &frozenState{selectionState: &selectionState{}, request: arrow, selected: 99}
	state.continueOrFinishDrawingRequest(arrow)
	if state.activeRequest() != arrow {
		t.Fatal("completed arrow placement deactivated the current tool")
	}
	if arrow.start != (image.Point{}) || arrow.current != (image.Point{}) {
		t.Fatalf("completed arrow geometry was not reset: %v -> %v", arrow.start, arrow.current)
	}
	if state.selected != 99 {
		t.Fatalf("completed arrow selection = %d, want 99", state.selected)
	}
	select {
	case err := <-arrow.result:
		t.Fatalf("completed arrow unexpectedly ended request: %v", err)
	default:
	}

	text := &frozenAnnotationRequest{tool: editor.ToolText, result: make(chan error, 1)}
	state.request = text
	state.selected = 99
	state.completeNewTextPlacement(nil, false)
	if state.activeRequest() != text {
		t.Fatal("completed text placement deactivated the current tool")
	}
	if state.selected != 99 {
		t.Fatalf("completed text selection = %d, want 99", state.selected)
	}
	select {
	case err := <-text.result:
		t.Fatalf("completed text unexpectedly ended request: %v", err)
	default:
	}
}

func TestDrawingToolSwitchAfterCapturedPlacementEndsCurrentRequest(t *testing.T) {
	request := &frozenAnnotationRequest{tool: editor.ToolRectangle, result: make(chan error, 1)}
	state := &frozenState{
		selectionState: &selectionState{}, request: request,
		toolSwapWait: true, toolSwapFrom: editor.ToolRectangle,
	}
	state.continueOrFinishDrawingRequest(request)
	if state.activeRequest() != nil || state.toolSwapWait {
		t.Fatal("pending tool switch did not end the completed rectangle request")
	}
	if err := <-request.result; err != nil {
		t.Fatalf("switched rectangle request completed with error: %v", err)
	}
}

func TestEarlyToolSwitchOnlyFinishesMatchingFutureRequest(t *testing.T) {
	state := &frozenState{selectionState: &selectionState{}}
	state.finishCurrentToolForSwitch(editor.ToolText)
	if !state.toolSwapWait || state.toolSwapFrom != editor.ToolText {
		t.Fatalf("early switch state = waiting:%v from:%v", state.toolSwapWait, state.toolSwapFrom)
	}

	rectangle := &frozenAnnotationRequest{tool: editor.ToolRectangle, result: make(chan error, 1)}
	state.request = rectangle
	if state.applyPendingToolSwitch() {
		t.Fatal("stale text switch cancelled a newer rectangle request")
	}
	if state.activeRequest() != rectangle || state.toolSwapWait {
		t.Fatal("mismatched request or pending switch state changed unexpectedly")
	}

	state.request = nil
	state.finishCurrentToolForSwitch(editor.ToolArrow)
	arrow := &frozenAnnotationRequest{tool: editor.ToolArrow, result: make(chan error, 1)}
	state.request = arrow
	if !state.applyPendingToolSwitch() {
		t.Fatal("early arrow switch did not finish the matching future request")
	}
	if state.activeRequest() != nil {
		t.Fatal("matching future request remained active")
	}
	if err := <-arrow.result; err != nil {
		t.Fatalf("matching future request completed with error: %v", err)
	}
}

func TestActiveDrawingToolsAcceptColorAndWidthBeforePlacement(t *testing.T) {
	for _, tool := range []editor.Tool{editor.ToolRectangle, editor.ToolArrow, editor.ToolText} {
		request := &frozenAnnotationRequest{tool: tool, style: editor.DefaultStyle(), result: make(chan error, 1)}
		state := &frozenState{selectionState: &selectionState{}, request: request}
		style := editor.Style{Color: editor.PresetColors()[2], Width: editor.PresetWidths()[3]}
		state.applyActiveToolStyle(tool, style)
		if request.style != style {
			t.Errorf("tool %v active style = %+v, want %+v", tool, request.style, style)
		}
	}
}

func TestActiveToolStyleUpdatesSelectedAnnotationAndFutureDrawing(t *testing.T) {
	document, err := editor.NewDocument(image.NewRGBA(image.Rect(0, 0, 100, 60)))
	if err != nil {
		t.Fatal(err)
	}
	original := editor.DefaultStyle()
	id, err := document.Add(editor.Annotation{Tool: editor.ToolArrow, Start: image.Pt(5, 20), End: image.Pt(80, 20), Style: original})
	if err != nil {
		t.Fatal(err)
	}
	request := &frozenAnnotationRequest{tool: editor.ToolRectangle, style: original}
	state := &frozenState{selectionState: &selectionState{}, document: document, request: request, selected: id}
	updated := editor.Style{Color: editor.PresetColors()[3], Width: editor.PresetWidths()[3]}
	state.applyActiveToolStyle(editor.ToolRectangle, updated)
	annotation, ok := document.Get(id)
	if !ok || annotation.Style != updated {
		t.Fatalf("selected annotation style = %+v, want %+v", annotation.Style, updated)
	}
	if request.style != updated {
		t.Fatalf("future drawing style = %+v, want %+v", request.style, updated)
	}
	if !document.Undo() {
		t.Fatal("selected style update was not added to undo history")
	}
}

func TestEarlyToolStyleOnlyAppliesToMatchingFutureRequest(t *testing.T) {
	style := editor.Style{Color: editor.PresetColors()[1], Width: editor.PresetWidths()[2]}
	state := &frozenState{selectionState: &selectionState{}}
	state.applyActiveToolStyle(editor.ToolArrow, style)
	if !state.toolStyleWait || state.toolStyleFor != editor.ToolArrow || state.toolStyle != style {
		t.Fatalf("early style state = waiting:%v tool:%v style:%+v", state.toolStyleWait, state.toolStyleFor, state.toolStyle)
	}

	request := &frozenAnnotationRequest{tool: editor.ToolArrow, style: editor.DefaultStyle(), result: make(chan error, 1)}
	state.request = request
	state.applyPendingToolStyle()
	if state.toolStyleWait || request.style != style {
		t.Fatalf("future arrow style = waiting:%v style:%+v, want %+v", state.toolStyleWait, request.style, style)
	}
}

func TestActiveToolbarStyleChoiceDoesNotEmitSecondAction(t *testing.T) {
	events := make(chan ToolbarEvent, 1)
	state := &toolbarState{
		persistent: true, active: true, activeAction: ActionRectangle,
		style: editor.DefaultStyle(), events: events,
		panel: stylePanel{field: editor.StyleFieldColor},
	}
	state.choosePanelOption(2)
	if state.style.Color != editor.PresetColors()[2] {
		t.Fatalf("active toolbar color = %+v, want %+v", state.style.Color, editor.PresetColors()[2])
	}
	if !state.active || state.ready {
		t.Fatalf("style choice changed active gate = active:%v ready:%v", state.active, state.ready)
	}
	select {
	case event := <-events:
		t.Fatalf("active style choice emitted unexpected event %+v", event)
	default:
	}

	state.panel.field = editor.StyleFieldWidth
	state.choosePanelOption(3)
	if state.style.Width != editor.PresetWidths()[3] {
		t.Fatalf("active toolbar width = %v, want %v", state.style.Width, editor.PresetWidths()[3])
	}
}

func TestToolStyleMessageRoundTrip(t *testing.T) {
	wantTool := editor.ToolText
	wantStyle := editor.Style{Color: editor.PresetColors()[4], Width: editor.PresetWidths()[1]}
	wParam, lParam := packToolStyle(wantTool, wantStyle)
	gotTool, gotStyle := unpackToolStyle(wParam, lParam)
	if gotTool != wantTool || gotStyle != wantStyle {
		t.Fatalf("unpacked tool/style = %v/%+v, want %v/%+v", gotTool, gotStyle, wantTool, wantStyle)
	}
}

func TestToolbarTooltipLabelsAreEnglish(t *testing.T) {
	state := &toolbarState{style: editor.DefaultStyle()}
	if got := state.tooltipLabel(ActionColor, "Color"); got != "Color: Red" {
		t.Fatalf("color tooltip = %q, want %q", got, "Color: Red")
	}
	if got := state.tooltipLabel(ActionWidth, "Line width"); got != "Line width: 3 px" {
		t.Fatalf("width tooltip = %q, want %q", got, "Line width: 3 px")
	}
	if got := state.tooltipLabel(ActionCopy, "Copy"); got != "Copy" {
		t.Fatalf("copy tooltip = %q, want %q", got, "Copy")
	}
}

func TestSelectedStyleUpdatePreservesOtherFieldAndIsUndoable(t *testing.T) {
	document, err := editor.NewDocument(image.NewRGBA(image.Rect(0, 0, 100, 60)))
	if err != nil {
		t.Fatal(err)
	}
	original := editor.DefaultStyle()
	original.Width = 8
	id, err := document.Add(editor.Annotation{Tool: editor.ToolRectangle, Start: image.Pt(5, 5), End: image.Pt(50, 30), Style: original})
	if err != nil {
		t.Fatal(err)
	}
	styles := make(chan editor.Style, 1)
	state := &frozenState{selectionState: &selectionState{}, document: document, selected: id, styleEvents: styles}
	green := editor.PresetColors()[2]
	result := state.applySelectedStyle(editor.StyleChange{Field: editor.StyleFieldColor, Style: editor.Style{Color: green}})
	if result.err != nil || !result.changed {
		t.Fatalf("style update result = %+v", result)
	}
	updated, _ := document.Get(id)
	if updated.Style.Color != green || updated.Style.Width != original.Width {
		t.Fatalf("updated style = %+v", updated.Style)
	}
	if notified := <-styles; notified != updated.Style {
		t.Fatalf("notified style = %+v, want %+v", notified, updated.Style)
	}
	if !document.Undo() {
		t.Fatal("style update did not create undo history")
	}
	restored, _ := document.Get(id)
	if restored.Style != original {
		t.Fatalf("restored style = %+v, want %+v", restored.Style, original)
	}
}

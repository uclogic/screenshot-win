package app

import (
	"context"
	"fmt"
	"image"
	"time"

	"screenshot-win/capture"
	"screenshot-win/editor"
	"screenshot-win/selector"
)

type interactiveOperations struct {
	showToolbar         func(image.Rectangle) (selector.Action, error)
	showToolbarEvent    func(image.Rectangle) (selector.ToolbarEvent, error)
	choosePath          func(time.Time) (string, bool, error)
	save                func(string, image.Image) error
	copy                func(image.Image) error
	pin                 func(image.Image, image.Point) error
	now                 func() time.Time
	showError           func(error)
	annotate            func(editor.Tool, editor.Style) error
	updateSelectedStyle func(context.Context, editor.StyleChange) (bool, error)
	rendered            func() image.Image
}

type actionResult struct {
	action selector.Action
	path   string
}

func (runner *Runner) runInteractive(ctx context.Context, config Config) error {
	session, err := runner.coordinator.Begin(StateSelecting)
	if err != nil {
		return err
	}
	defer session.Finish()

	fmt.Fprintln(runner.runtime.Stdout, "请拖动鼠标选择截图区域；按 Esc 或右键取消。")
	region, selected, err := selector.SelectContext(ctx)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !selected {
		fmt.Fprintln(runner.runtime.Stdout, "已取消截图。")
		return nil
	}
	config.X, config.Y = region.Min.X, region.Min.Y
	config.Width, config.Height = region.Dx(), region.Dy()

	// Capture the entire virtual desktop so the post-selection UI can block
	// interaction while showing one consistent frozen frame.
	desktop := selector.DesktopBounds()
	if desktop.Empty() {
		return fmt.Errorf("virtual desktop has invalid bounds %v", desktop)
	}
	desktopSnapshot, err := capture.Region(desktop.Min.X, desktop.Min.Y, desktop.Dx(), desktop.Dy())
	if err != nil {
		return err
	}
	localRegion := region.Sub(desktop.Min)
	snapshot := desktopSnapshot.SubImage(localRegion)
	frozen, err := selector.ShowFrozenDesktop(desktopSnapshot, region)
	if err != nil {
		return err
	}
	if err := session.Transition(StateSelecting, StateFrozen); err != nil {
		frozen.Close()
		return err
	}
	toolbar, err := selector.ShowToolbarContext(ctx, region, frozen.WindowHandle())
	if err != nil {
		frozen.Close()
		return err
	}
	stopStyleSync := syncSelectedStyles(ctx, toolbar, frozen.SelectedStyles())
	defer stopStyleSync()

	result, err := runActionMenu(region, snapshot, interactiveOperations{
		showToolbarEvent: func(region image.Rectangle) (selector.ToolbarEvent, error) {
			return toolbar.NextEvent(ctx)
		},
		choosePath: func(now time.Time) (string, bool, error) {
			return runner.choosePNGPath(ctx, frozen.WindowHandle(), now)
		},
		save:     savePNG,
		copy:     capture.CopyImage,
		pin:      runner.pinImage,
		annotate: func(tool editor.Tool, style editor.Style) error { return frozen.AnnotateContext(ctx, tool, style) },
		updateSelectedStyle: func(updateCtx context.Context, change editor.StyleChange) (bool, error) {
			return frozen.UpdateSelectedStyleContext(updateCtx, change)
		},
		rendered: frozen.Rendered,
		now:      runner.runtime.Now,
		showError: func(err error) {
			fmt.Fprintln(runner.runtime.Stderr, "screenshot-win:", err)
			if runner.runtime.ShowError != nil {
				runner.runtime.ShowError(frozen.WindowHandle(), err)
			}
		},
	})
	toolbar.Close()
	frozen.Close()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	switch result.action {
	case selector.ActionSave:
		fmt.Fprintf(runner.runtime.Stdout, "已保存 %s（%d × %d）\n", result.path, snapshot.Bounds().Dx(), snapshot.Bounds().Dy())
		return nil
	case selector.ActionCopy:
		fmt.Fprintf(runner.runtime.Stdout, "已复制截图到剪贴板（%d × %d）\n", snapshot.Bounds().Dx(), snapshot.Bounds().Dy())
		return nil
	case selector.ActionScroll:
		if err := session.Transition(StateFrozen, StateScrolling); err != nil {
			return err
		}
		return runner.runLongCapture(ctx, config, true, session)
	case selector.ActionPin:
		fmt.Fprintf(runner.runtime.Stdout, "已将截图贴到桌面（%d × %d）\n", snapshot.Bounds().Dx(), snapshot.Bounds().Dy())
		return nil
	default:
		fmt.Fprintln(runner.runtime.Stdout, "已取消截图。")
		return nil
	}
}

func runActionMenu(region image.Rectangle, snapshot image.Image, operations interactiveOperations) (actionResult, error) {
	style := editor.DefaultStyle()
	for {
		event := selector.ToolbarEvent{Style: style}
		var err error
		if operations.showToolbarEvent != nil {
			event, err = operations.showToolbarEvent(region)
		} else if operations.showToolbar != nil {
			event.Action, err = operations.showToolbar(region)
		} else {
			return actionResult{}, errorsMissingOperation("show toolbar")
		}
		if err != nil {
			return actionResult{}, err
		}
		action := event.Action
		if event.Style.Width > 0 {
			style = event.Style
		}
		switch action {
		case selector.ActionSave:
			output := currentEditedImage(snapshot, operations.rendered)
			path, selected, err := operations.choosePath(operations.now())
			if err != nil {
				operations.showError(err)
				continue
			}
			if !selected {
				continue
			}
			if err := operations.save(path, output); err != nil {
				operations.showError(err)
				continue
			}
			return actionResult{action: action, path: path}, nil
		case selector.ActionCopy:
			if err := operations.copy(currentEditedImage(snapshot, operations.rendered)); err != nil {
				operations.showError(err)
				continue
			}
			return actionResult{action: action}, nil
		case selector.ActionScroll:
			return actionResult{action: action}, nil
		case selector.ActionRectangle, selector.ActionArrow, selector.ActionText:
			if operations.annotate == nil {
				return actionResult{}, errorsMissingOperation("annotate image")
			}
			tool := editor.ToolRectangle
			if action == selector.ActionArrow {
				tool = editor.ToolArrow
			}
			if action == selector.ActionText {
				tool = editor.ToolText
			}
			if err := operations.annotate(tool, style); err != nil && operations.showError != nil {
				operations.showError(err)
			}
			continue
		case selector.ActionColor:
			if event.Change.Field == editor.StyleFieldColor && operations.updateSelectedStyle != nil {
				if _, err := operations.updateSelectedStyle(context.Background(), event.Change); err != nil && operations.showError != nil {
					operations.showError(err)
				}
			}
			continue
		case selector.ActionWidth:
			if event.Change.Field == editor.StyleFieldWidth && operations.updateSelectedStyle != nil {
				if _, err := operations.updateSelectedStyle(context.Background(), event.Change); err != nil && operations.showError != nil {
					operations.showError(err)
				}
			}
			continue
		case selector.ActionPin:
			if operations.pin == nil {
				return actionResult{}, errorsMissingOperation("pin image")
			}
			if err := operations.pin(currentEditedImage(snapshot, operations.rendered), region.Min); err != nil {
				operations.showError(err)
				continue
			}
			return actionResult{action: action}, nil
		default:
			return actionResult{action: selector.ActionCancel}, nil
		}
	}
}

func syncSelectedStyles(ctx context.Context, toolbar *selector.ActionToolbar, styles <-chan editor.Style) func() {
	syncCtx, cancel := context.WithCancel(ctx)
	if toolbar == nil || styles == nil {
		return cancel
	}
	go func() {
		for {
			select {
			case style, ok := <-styles:
				if !ok {
					return
				}
				toolbar.SetStyle(style)
			case <-syncCtx.Done():
				return
			}
		}
	}()
	return cancel
}

func currentEditedImage(original image.Image, rendered func() image.Image) image.Image {
	if rendered != nil {
		if result := rendered(); result != nil {
			return result
		}
	}
	return original
}

func errorsMissingOperation(name string) error {
	return fmt.Errorf("runtime operation %q is not configured", name)
}

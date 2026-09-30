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

func (runner *Runner) runInlineAnnotation(ctx context.Context, source image.Image, region image.Rectangle, layout []string) error {
	result, output, err := runner.editInlineAnnotation(ctx, source, region, layout, runner.pinImage, 0)
	if err != nil {
		return err
	}
	size := output.Bounds().Size()
	switch result.action {
	case selector.ActionSave:
		fmt.Fprintf(runner.runtime.Stdout, "已保存编辑结果 %s（%d × %d）\n", result.path, size.X, size.Y)
	case selector.ActionCopy:
		fmt.Fprintf(runner.runtime.Stdout, "已复制编辑结果到剪贴板（%d × %d）\n", size.X, size.Y)
	case selector.ActionPin:
		fmt.Fprintf(runner.runtime.Stdout, "已将编辑结果贴到桌面（%d × %d）\n", size.X, size.Y)
	default:
		fmt.Fprintln(runner.runtime.Stdout, "已取消标注，不会创建输出文件。")
	}
	return nil
}

func (runner *Runner) editInlineAnnotation(ctx context.Context, source image.Image, region image.Rectangle, layout []string, pin func(image.Image, image.Point) error, scale float64) (actionResult, image.Image, error) {
	if err := ctx.Err(); err != nil {
		return actionResult{}, nil, err
	}
	desktop := selector.DesktopBounds()
	if desktop.Empty() {
		return actionResult{}, nil, fmt.Errorf("virtual desktop has invalid bounds %v", desktop)
	}
	desktopSnapshot, err := capture.Region(desktop.Min.X, desktop.Min.Y, desktop.Dx(), desktop.Dy())
	if err != nil {
		return actionResult{}, nil, err
	}
	frozen, err := selector.ShowFrozenContent(desktopSnapshot, region, source, scale)
	if err != nil {
		return actionResult{}, nil, err
	}
	defer frozen.Close()
	toolbar, err := selector.ShowAnnotationToolbarContextWithLayout(ctx, region, frozen.WindowHandle(), layout, frozen.Background)
	if err != nil {
		return actionResult{}, nil, err
	}
	defer toolbar.Close()
	stopStyleSync := syncSelectedStyles(ctx, toolbar, frozen.SelectedStyles())
	defer stopStyleSync()
	result, err := runActionMenu(region, source, interactiveOperations{
		showToolbarEvent: func(region image.Rectangle) (selector.ToolbarEvent, error) {
			return toolbar.NextEvent(ctx)
		},
		choosePath: func(now time.Time) (string, bool, error) {
			return runner.choosePNGPath(ctx, frozen.WindowHandle(), now)
		},
		save: savePNG, copy: capture.CopyImage, pin: pin, now: runner.runtime.Now,
		showError: func(err error) {
			fmt.Fprintln(runner.runtime.Stderr, "screenshot-win:", err)
			if runner.runtime.ShowError != nil {
				runner.runtime.ShowError(frozen.WindowHandle(), err)
			}
		},
		annotate: func(tool editor.Tool, style editor.Style) error { return frozen.AnnotateContext(ctx, tool, style) }, rendered: frozen.Rendered,
		updateSelectedStyle: func(updateCtx context.Context, change editor.StyleChange) (bool, error) {
			return frozen.UpdateSelectedStyleContext(updateCtx, change)
		},
	})
	if err != nil {
		return actionResult{}, nil, err
	}
	if err := ctx.Err(); err != nil {
		return actionResult{}, nil, err
	}
	return result, frozen.Rendered(), nil
}

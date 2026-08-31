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

func (runner *Runner) runInlineAnnotation(ctx context.Context, source image.Image, region image.Rectangle) error {
	desktop := selector.DesktopBounds()
	if desktop.Empty() {
		return fmt.Errorf("virtual desktop has invalid bounds %v", desktop)
	}
	desktopSnapshot, err := capture.Region(desktop.Min.X, desktop.Min.Y, desktop.Dx(), desktop.Dy())
	if err != nil {
		return err
	}
	frozen, err := selector.ShowFrozenContent(desktopSnapshot, region, source)
	if err != nil {
		return err
	}
	defer frozen.Close()
	toolbar, err := selector.ShowAnnotationToolbarContext(ctx, region, frozen.WindowHandle())
	if err != nil {
		return err
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
		save: savePNG, copy: capture.CopyImage, pin: runner.pinImage, now: runner.runtime.Now,
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
		return err
	}
	output := frozen.Rendered()
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

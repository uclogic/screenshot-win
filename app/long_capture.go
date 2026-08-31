package app

import (
	"context"
	"fmt"
	"image"
	"os"
	"os/signal"
	"time"

	"screenshot-win"
	"screenshot-win/capture"
	"screenshot-win/selector"
)

type runStats struct {
	captured       int
	analyzed       int
	matched        int
	appended       int
	rejected       map[screenshotwin.RejectionReason]int
	diagnosticDrop int
}

type longCapturePreview interface {
	Update(image.Image) error
	Close()
}

type longCaptureVisibility interface {
	HideForCapture()
	RestoreAfterCapture()
}

type pngPathChooser func(uintptr, time.Time) (string, bool, error)

type longCaptureActionDecision struct {
	finish bool
	action selector.Action
	path   string
}

func (runner *Runner) runLongCapture(ctx context.Context, config Config, showBorder bool, session *Session) error {
	region := image.Rect(config.X, config.Y, config.X+config.Width, config.Y+config.Height)
	var border *selector.Border
	if showBorder {
		var borderErr error
		border, borderErr = selector.ShowBorder(region)
		if borderErr != nil {
			fmt.Fprintf(runner.runtime.Stderr, "警告：无法显示截图区域边框：%v\n", borderErr)
		} else {
			defer border.Close()
		}
	}
	shield, shieldErr := selector.ShowMouseShield(region)
	if shieldErr != nil {
		fmt.Fprintf(runner.runtime.Stderr, "警告：无法阻止截图区域的鼠标悬停事件：%v\n", shieldErr)
	}
	defer func() {
		if shield != nil {
			shield.Close()
		}
	}()

	diagnostics, diagnosticErr := newDiagnosticWriter(config.DiagnosticDir, config.DiagnosticMax)
	if diagnosticErr != nil {
		fmt.Fprintf(runner.runtime.Stderr, "警告：诊断已停用：%v\n", diagnosticErr)
	}

	previous, err := capture.Region(config.X, config.Y, config.Width, config.Height)
	if err != nil {
		closeDiagnostics(diagnostics)
		return err
	}
	matcher, err := screenshotwin.NewMatcher(previous, config.MatchOptions)
	if err != nil {
		closeDiagnostics(diagnostics)
		return err
	}
	builder, err := screenshotwin.NewBuilder(previous)
	if err != nil {
		closeDiagnostics(diagnostics)
		return err
	}
	var preview longCapturePreview
	if longCapturePreviewEnabled(config) {
		preview, err = selector.ShowPreview(region, builder.Image())
		if err != nil {
			fmt.Fprintf(runner.runtime.Stderr, "警告：长截图缩略图已停用：%v\n", err)
			preview = nil
		} else if preview != nil {
			defer preview.Close()
		}
	}
	var captureToolbar *selector.CaptureToolbar
	var captureActions <-chan selector.Action
	if config.Interactive {
		captureToolbar, err = selector.ShowCaptureToolbar(region)
		if err != nil {
			fmt.Fprintf(runner.runtime.Stderr, "警告：长截图控制栏无法显示：%v\n", err)
			captureToolbar = nil
		} else {
			captureActions = captureToolbar.Actions()
			defer captureToolbar.Close()
		}
	}
	stats := runStats{captured: 1, rejected: make(map[screenshotwin.RejectionReason]int)}
	fmt.Fprintln(runner.runtime.Stdout, "开始截图。请缓慢向下滚动；按 Esc 或 Ctrl+C 结束。")

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()
	finishAction := defaultLongCaptureFinishAction(config)
	finishPath := ""

captureLoop:
	for {
		select {
		case <-ctx.Done():
			finishAction = selector.ActionCancel
			break captureLoop
		case <-interrupts:
			break captureLoop
		case action, ok := <-captureActions:
			if !ok {
				captureActions = nil
				continue
			}
			if captureToolbar != nil {
				captureToolbar.Close()
				captureToolbar = nil
				captureActions = nil
			}
			if shield != nil {
				shield.Close()
				shield = nil
			}
			if action == selector.ActionSaveAs && preview != nil {
				preview.Close()
				preview = nil
			}
			owner := uintptr(0)
			if border != nil {
				owner = border.WindowHandle()
			}
			decision, actionErr := decideLongCaptureAction(action, owner, runner.runtime.Now(), func(owner uintptr, now time.Time) (string, bool, error) {
				return runner.choosePNGPath(ctx, owner, now)
			})
			if actionErr != nil {
				closeDiagnostics(diagnostics)
				return actionErr
			}
			if !decision.finish {
				fmt.Fprintln(runner.runtime.Stdout, "已取消另存，继续长截图。")
				shield, shieldErr = selector.ShowMouseShield(region)
				if shieldErr != nil {
					fmt.Fprintf(runner.runtime.Stderr, "警告：无法阻止截图区域的鼠标悬停事件：%v\n", shieldErr)
					shield = nil
				}
				preview, err = selector.ShowPreview(region, builder.Image())
				if err != nil {
					fmt.Fprintf(runner.runtime.Stderr, "警告：长截图缩略图无法恢复：%v\n", err)
					preview = nil
				}
				captureToolbar, err = selector.ShowCaptureToolbar(region)
				if err != nil {
					fmt.Fprintf(runner.runtime.Stderr, "警告：长截图控制栏无法恢复：%v\n", err)
					captureToolbar = nil
				} else {
					captureActions = captureToolbar.Actions()
				}
				ticker.Reset(config.Interval)
				continue
			}
			finishAction = decision.action
			finishPath = decision.path
			break captureLoop
		case capturedAt := <-ticker.C:
			if capture.EscapePressed() {
				break captureLoop
			}
			current, captureErr := captureWithLongCaptureOverlays(func() (*image.RGBA, error) {
				return capture.Region(config.X, config.Y, config.Width, config.Height)
			}, preview, captureToolbar)
			if captureErr != nil {
				closeDiagnostics(diagnostics)
				return captureErr
			}
			stats.captured++
			result, analyzeErr := matcher.Analyze(current)
			if analyzeErr != nil {
				closeDiagnostics(diagnostics)
				return analyzeErr
			}
			stats.analyzed++

			if diagnostics != nil {
				if submitErr := diagnostics.submit(stats.analyzed, capturedAt, result, previous, current); submitErr != nil {
					fmt.Fprintf(runner.runtime.Stderr, "警告：诊断已停用：%v\n", submitErr)
					stats.diagnosticDrop += diagnostics.droppedCount()
					diagnostics.close()
					diagnostics = nil
				}
			}

			if !result.Matched {
				stats.rejected[result.Reason]++
				continue
			}
			if appendErr := builder.Append(current, result.Offset); appendErr != nil {
				closeDiagnostics(diagnostics)
				return appendErr
			}
			if preview != nil {
				var previewErr error
				preview, previewErr = refreshLongCapturePreview(preview, builder.Image())
				if previewErr != nil {
					fmt.Fprintf(runner.runtime.Stderr, "警告：长截图缩略图已停用：%v\n", previewErr)
				}
			}
			previous = current
			stats.matched++
			stats.appended += result.Offset
			fmt.Fprintf(runner.runtime.Stdout, "已追加 %d px，总高度 %d px（评分 %.3f）\n", result.Offset, builder.Height(), result.BestScore)
		}
	}
	if captureToolbar != nil {
		captureToolbar.Close()
		captureToolbar = nil
	}
	if preview != nil {
		preview.Close()
		preview = nil
	}
	if shield != nil {
		shield.Close()
		shield = nil
	}

	if diagnostics != nil {
		stats.diagnosticDrop += diagnostics.droppedCount()
		if closeErr := diagnostics.close(); closeErr != nil {
			fmt.Fprintf(runner.runtime.Stderr, "警告：诊断写入不完整：%v\n", closeErr)
		}
	}
	if finishAction == selector.ActionCancel {
		fmt.Fprintln(runner.runtime.Stdout, "已取消长截图，不会创建输出文件。")
		return nil
	}
	output := builder.Finish()
	switch finishAction {
	case selector.ActionEdit:
		if session == nil {
			return errorsMissingOperation("editing session")
		}
		if err := session.Transition(StateScrolling, StateEditing); err != nil {
			return err
		}
		if border != nil {
			border.Close()
			border = nil
		}
		return runner.runInlineAnnotation(ctx, output, region)
	case selector.ActionSaveAs:
		if err := savePNG(finishPath, output); err != nil {
			return err
		}
		runner.printSummary(stats, output, finishPath)
		return nil
	case selector.ActionCopy:
		if err := capture.CopyImage(output); err != nil {
			return err
		}
		fmt.Fprintf(runner.runtime.Stdout, "已复制长截图到剪贴板（%d × %d）\n", output.Bounds().Dx(), output.Bounds().Dy())
		runner.printRunStats(stats)
		return nil
	case selector.ActionPin:
		if err := runner.pinImage(output, image.Pt(config.X, config.Y)); err != nil {
			return err
		}
		fmt.Fprintf(runner.runtime.Stdout, "已将长截图贴到桌面（%d × %d）\n", output.Bounds().Dx(), output.Bounds().Dy())
		runner.printRunStats(stats)
		return nil
	default:
		if err := savePNG(config.OutputPath, output); err != nil {
			return err
		}
		runner.printSummary(stats, output, config.OutputPath)
		return nil
	}
}

func captureWithLongCaptureOverlays(captureFrame func() (*image.RGBA, error), candidates ...any) (*image.RGBA, error) {
	overlays := make([]longCaptureVisibility, 0, len(candidates))
	for _, candidate := range candidates {
		if overlay, ok := candidate.(longCaptureVisibility); ok && overlay != nil {
			overlay.HideForCapture()
			overlays = append(overlays, overlay)
		}
	}
	defer func() {
		for index := len(overlays) - 1; index >= 0; index-- {
			overlays[index].RestoreAfterCapture()
		}
	}()
	return captureFrame()
}

func decideLongCaptureAction(action selector.Action, owner uintptr, now time.Time, choose pngPathChooser) (longCaptureActionDecision, error) {
	decision := longCaptureActionDecision{finish: true, action: action}
	if action != selector.ActionSaveAs {
		return decision, nil
	}
	if choose == nil {
		return longCaptureActionDecision{}, errorsMissingOperation("choose PNG path")
	}
	path, selected, err := choose(owner, now)
	if err != nil {
		return longCaptureActionDecision{}, err
	}
	if !selected {
		return longCaptureActionDecision{}, nil
	}
	decision.path = path
	return decision, nil
}

func defaultLongCaptureFinishAction(config Config) selector.Action {
	if config.Interactive {
		return selector.ActionCancel
	}
	return selector.ActionSave
}

func longCapturePreviewEnabled(config Config) bool { return config.Interactive }

func refreshLongCapturePreview(preview longCapturePreview, source image.Image) (longCapturePreview, error) {
	if preview == nil {
		return nil, nil
	}
	if err := preview.Update(source); err != nil {
		preview.Close()
		return nil, err
	}
	return preview, nil
}

func closeDiagnostics(diagnostics *diagnosticWriter) {
	if diagnostics != nil {
		diagnostics.close()
	}
}

func (runner *Runner) printSummary(stats runStats, output image.Image, outputPath string) {
	fmt.Fprintf(runner.runtime.Stdout, "已保存 %s（%d × %d）\n", outputPath, output.Bounds().Dx(), output.Bounds().Dy())
	runner.printRunStats(stats)
}

func (runner *Runner) printRunStats(stats runStats) {
	fmt.Fprintf(runner.runtime.Stdout, "摘要：捕获 %d 帧，分析 %d 次，成功 %d 次，追加 %d px\n", stats.captured, stats.analyzed, stats.matched, stats.appended)
	fmt.Fprintf(runner.runtime.Stdout, "拒绝：静止 %d，评分过高 %d，结果歧义 %d，其他 %d\n",
		stats.rejected[screenshotwin.RejectionStationary],
		stats.rejected[screenshotwin.RejectionScoreTooHigh],
		stats.rejected[screenshotwin.RejectionAmbiguous],
		otherRejections(stats.rejected),
	)
	if stats.diagnosticDrop > 0 {
		fmt.Fprintf(runner.runtime.Stdout, "诊断队列丢弃 %d 条记录\n", stats.diagnosticDrop)
	}
}

func otherRejections(rejected map[screenshotwin.RejectionReason]int) int {
	total := 0
	for reason, count := range rejected {
		if reason != screenshotwin.RejectionStationary && reason != screenshotwin.RejectionScoreTooHigh && reason != screenshotwin.RejectionAmbiguous {
			total += count
		}
	}
	return total
}

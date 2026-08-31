package app

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"time"

	"screenshot-win"
)

const diagnosticQueueSize = 32

type diagnosticEvent struct {
	Sequence        int                           `json:"sequence"`
	CapturedAt      time.Time                     `json:"captured_at"`
	Implementation  string                        `json:"implementation"`
	Matched         bool                          `json:"matched"`
	Offset          int                           `json:"offset"`
	Position        int                           `json:"position"`
	AddedTop        int                           `json:"added_top"`
	AddedBottom     int                           `json:"added_bottom"`
	Relocalized     bool                          `json:"relocalized"`
	BestScore       float64                       `json:"best_score"`
	SecondBestScore float64                       `json:"second_best_score"`
	Reason          screenshotwin.RejectionReason `json:"reason,omitempty"`
	PreviousImage   string                        `json:"previous_image,omitempty"`
	CurrentImage    string                        `json:"current_image,omitempty"`
}

type diagnosticJob struct {
	event    diagnosticEvent
	previous image.Image
	current  image.Image
}

type diagnosticWriter struct {
	directory     string
	file          *os.File
	jobs          chan diagnosticJob
	maxRejected   int
	savedRejected int
	dropped       int
	wait          sync.WaitGroup
	errMu         sync.Mutex
	err           error
	closed        bool
}

func newDiagnosticWriter(directory string, maxRejected int) (*diagnosticWriter, error) {
	if directory == "" {
		return nil, nil
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("create diagnostic directory: %w", err)
	}
	file, err := os.Create(filepath.Join(directory, "events.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("create diagnostic event log: %w", err)
	}
	writer := &diagnosticWriter{
		directory:   directory,
		file:        file,
		jobs:        make(chan diagnosticJob, diagnosticQueueSize),
		maxRejected: maxRejected,
	}
	writer.wait.Add(1)
	go writer.run()
	return writer, nil
}

func (writer *diagnosticWriter) submit(sequence int, capturedAt time.Time, implementation LongCaptureImplementation, result longCaptureFrameResult, previous, current image.Image) error {
	if err := writer.currentError(); err != nil {
		return err
	}
	event := diagnosticEvent{
		Sequence:        sequence,
		CapturedAt:      capturedAt,
		Implementation:  implementation.String(),
		Matched:         result.matched,
		Offset:          result.offset,
		Position:        result.position,
		AddedTop:        result.addedTop,
		AddedBottom:     result.addedBottom,
		Relocalized:     result.relocalized,
		BestScore:       result.bestScore,
		SecondBestScore: result.secondBestScore,
		Reason:          result.reason,
	}
	job := diagnosticJob{event: event}
	saveImages := !result.matched && result.reason != screenshotwin.RejectionStationary && writer.savedRejected < writer.maxRejected
	if saveImages {
		index := writer.savedRejected + 1
		job.event.PreviousImage = fmt.Sprintf("rejected-%06d-previous.png", index)
		job.event.CurrentImage = fmt.Sprintf("rejected-%06d-current.png", index)
		job.previous = previous
		job.current = current
	}
	select {
	case writer.jobs <- job:
		if saveImages {
			writer.savedRejected++
		}
	default:
		writer.dropped++
	}
	return nil
}

func (writer *diagnosticWriter) run() {
	defer writer.wait.Done()
	encoder := json.NewEncoder(writer.file)
	for job := range writer.jobs {
		if writer.currentError() != nil {
			continue
		}
		if job.previous != nil {
			if err := writeDiagnosticPNG(filepath.Join(writer.directory, job.event.PreviousImage), job.previous); err != nil {
				writer.setError(err)
				continue
			}
			if err := writeDiagnosticPNG(filepath.Join(writer.directory, job.event.CurrentImage), job.current); err != nil {
				writer.setError(err)
				continue
			}
		}
		if err := encoder.Encode(job.event); err != nil {
			writer.setError(fmt.Errorf("write diagnostic event: %w", err))
		}
	}
}

func (writer *diagnosticWriter) close() error {
	if writer == nil || writer.closed {
		return nil
	}
	writer.closed = true
	close(writer.jobs)
	writer.wait.Wait()
	if err := writer.file.Close(); err != nil && writer.currentError() == nil {
		writer.setError(fmt.Errorf("close diagnostic event log: %w", err))
	}
	return writer.currentError()
}

func (writer *diagnosticWriter) droppedCount() int {
	if writer == nil {
		return 0
	}
	return writer.dropped
}

func (writer *diagnosticWriter) currentError() error {
	writer.errMu.Lock()
	defer writer.errMu.Unlock()
	return writer.err
}

func (writer *diagnosticWriter) setError(err error) {
	writer.errMu.Lock()
	defer writer.errMu.Unlock()
	if writer.err == nil {
		writer.err = err
	}
}

func writeDiagnosticPNG(path string, source image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create diagnostic image: %w", err)
	}
	if err := png.Encode(file, source); err != nil {
		file.Close()
		return fmt.Errorf("encode diagnostic image: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close diagnostic image: %w", err)
	}
	return nil
}

package app

import (
	"bufio"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"screenshot-win"
)

func TestDiagnosticWriterRecordsEventsAndLimitsImages(t *testing.T) {
	directory := t.TempDir()
	writer, err := newDiagnosticWriter(directory, 1)
	if err != nil {
		t.Fatal(err)
	}
	frame := image.NewRGBA(image.Rect(0, 0, 20, 20))
	result := longCaptureFrameResult{
		offset:          -7,
		position:        -20,
		bestScore:       12,
		secondBestScore: 13,
		reason:          screenshotwin.RejectionScoreTooHigh,
	}
	if err := writer.submit(1, time.Unix(1, 0), LongCaptureBidirectional, result, frame, frame); err != nil {
		t.Fatal(err)
	}
	if err := writer.submit(2, time.Unix(2, 0), LongCaptureBidirectional, result, frame, frame); err != nil {
		t.Fatal(err)
	}
	if err := writer.close(); err != nil {
		t.Fatal(err)
	}

	file, err := os.Open(filepath.Join(directory, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var events []diagnosticEvent
	for scanner.Scan() {
		var event diagnosticEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Implementation != "bidirectional" || events[0].Offset != -7 || events[0].Position != -20 {
		t.Fatalf("unexpected bidirectional diagnostic fields: %+v", events[0])
	}
	if events[0].PreviousImage == "" || events[1].PreviousImage != "" {
		t.Fatalf("unexpected diagnostic image references: %+v", events)
	}
	for _, name := range []string{"rejected-000001-previous.png", "rejected-000001-current.png"} {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			t.Errorf("expected %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(directory, "rejected-000002-previous.png")); !os.IsNotExist(err) {
		t.Fatalf("second rejected pair should not have been written: %v", err)
	}
}

func TestDiagnosticWriterDisabledWithoutDirectory(t *testing.T) {
	writer, err := newDiagnosticWriter("", 50)
	if err != nil || writer != nil {
		t.Fatalf("newDiagnosticWriter() = (%v, %v), want (nil, nil)", writer, err)
	}
}

func TestDiagnosticWriterReportsAsynchronousFailure(t *testing.T) {
	writer, err := newDiagnosticWriter(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	want := os.ErrPermission
	writer.setError(want)
	if err := writer.submit(1, time.Now(), LongCaptureLegacy, longCaptureFrameResult{}, nil, nil); err != want {
		t.Fatalf("submit() error = %v, want %v", err, want)
	}
	writer.close()
}

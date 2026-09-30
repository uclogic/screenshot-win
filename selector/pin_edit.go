package selector

import (
	"context"
	"image"
)

type pinEditResult struct {
	image image.Image
	err   error
}

type pinEditSession struct {
	cancel context.CancelFunc
	done   chan struct{}
	result chan pinEditResult
}

func startPinEdit(edit PinEditor, source image.Image, bounds image.Rectangle, notify func()) *pinEditSession {
	ctx, cancel := context.WithCancel(context.Background())
	session := &pinEditSession{cancel: cancel, done: make(chan struct{}), result: make(chan pinEditResult, 1)}
	go func() {
		defer close(session.done)
		output, err := edit(ctx, source, bounds)
		if ctx.Err() != nil {
			output, err = nil, ctx.Err()
		}
		session.result <- pinEditResult{image: output, err: err}
		if ctx.Err() == nil {
			notify()
		}
	}()
	return session
}

func (session *pinEditSession) close() {
	if session != nil {
		session.cancel()
		<-session.done
	}
}

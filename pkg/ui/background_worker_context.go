package ui

import (
	"context"
)

// buildContextLocked is called under w.mu. Stop cancels the worker context,
// recovery cancels the old loop context, and the existing processing budget
// limits each admitted build. No mutable "current read" context is shared by
// overlapping old/new generations. Callers must release the returned context.
func (w *BackgroundWorker) buildContextLocked() (context.Context, context.CancelFunc) {
	parent := w.loopCtx
	if parent == nil {
		parent = w.ctx
	}
	if parent == nil {
		parent = context.Background()
	}
	if w.processingTimeout > 0 {
		return context.WithTimeout(parent, w.processingTimeout)
	}
	return context.WithCancel(parent)
}

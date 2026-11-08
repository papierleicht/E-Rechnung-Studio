package app

import "sync/atomic"

// closeGuard mirrors the frontend's unsaved state for native window-close events.
// The prompt flag prevents repeated close clicks from opening multiple dialogs.
type closeGuard struct {
	dirty      atomic.Bool
	promptOpen atomic.Bool
}

func (g *closeGuard) setDirty(dirty bool) {
	g.dirty.Store(dirty)
}

func (g *closeGuard) needsConfirmation() bool {
	return g.dirty.Load()
}

func (g *closeGuard) beginPrompt() bool {
	return g.promptOpen.CompareAndSwap(false, true)
}

func (g *closeGuard) finishPrompt(discard bool) {
	if discard {
		g.dirty.Store(false)
	}
	g.promptOpen.Store(false)
}

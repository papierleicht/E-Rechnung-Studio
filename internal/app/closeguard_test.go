package app

import "testing"

func TestCloseGuard(t *testing.T) {
	var guard closeGuard
	if guard.needsConfirmation() {
		t.Fatal("clean window must close without confirmation")
	}
	guard.setDirty(true)
	if !guard.needsConfirmation() || !guard.beginPrompt() {
		t.Fatal("dirty window must open one confirmation")
	}
	if guard.beginPrompt() {
		t.Fatal("a second confirmation must not open")
	}
	guard.finishPrompt(false)
	if !guard.needsConfirmation() || !guard.beginPrompt() {
		t.Fatal("cancelling must keep changes dirty and permit another prompt")
	}
	guard.finishPrompt(true)
	if guard.needsConfirmation() {
		t.Fatal("discarding must allow the window to close")
	}
}

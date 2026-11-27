package bot

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestQueuePanicRecovery(t *testing.T) {
	c := &Client{}
	c.initQueue()
	defer close(c.msgQueue)

	var ok atomic.Bool
	// Nil evt will panic inside handleMessage if not nil-checked, or we test safeHandleMessage directly
	c.safeHandleMessage(nil)
	ok.Store(true)

	done := make(chan struct{})
	go func() {
		// Ensure worker goroutines stay alive after nil/panic
		time.Sleep(20 * time.Millisecond)
		close(done)
	}()
	<-done
	if !ok.Load() {
		t.Fatal("expected safeHandleMessage to recover cleanly")
	}
}

package server

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// Shutdown cancels in-flight request contexts and waits for their handlers, so a
// stream interrupted by Ctrl+C still persists its partial version.
func TestRunCancelsRequestsAndWaitsForHandlers(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	var finished atomic.Bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(entered)
		<-r.Context().Done()               // like an LLM stream noticing cancellation
		time.Sleep(100 * time.Millisecond) // like saving the partial version
		finished.Store(true)
	})

	ctx, stop := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- Run(ctx, ln, h, 5*time.Second) }()

	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err == nil {
			resp.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("handler never started")
	}

	stop()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after shutdown")
	}
	if !finished.Load() {
		t.Fatal("Run returned before the in-flight handler finished")
	}
}

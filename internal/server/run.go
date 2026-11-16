package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// Run serves h on ln until ctx is cancelled (Ctrl+C / SIGTERM), then shuts down
// gracefully: every request context is cancelled — so LLM streams stop and the service
// stores the partial prompt as an incomplete version — and in-flight handlers get up
// to grace to finish before the process exits.
//
// There is deliberately no WriteTimeout: SSE responses legitimately stream for minutes
// (LLM streams are bounded by the provider idle timeout instead).
func Run(ctx context.Context, ln net.Listener, h http.Handler, grace time.Duration) error {
	baseCtx, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()

	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return baseCtx },
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	cancelRequests()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	err := srv.Shutdown(shutdownCtx)
	if serr := <-serveErr; serr != nil && !errors.Is(serr, http.ErrServerClosed) && err == nil {
		err = serr
	}
	return err
}

package fills

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Server receives the fill reports a broker pushes.
//
// The broker's webhook URL has to point at it: the proxy posts to
// {WEBHOOK_URL}/fills, and the proxy's default webhook URL is
// http://127.0.0.1:8000/webhook.
type Server struct {
	addr string
	srv  *http.Server
	log  Logger
	ln   net.Listener
}

// NewServer returns a server that listens on addr for fill reports.
//
// The registry names the orders the fills belong to.
func NewServer(addr string, reg *Registry, log Logger) *Server {
	mux := http.NewServeMux()
	mux.Handle("/", reg.Handler(log))
	return &Server{
		addr: addr,
		log:  log,
		srv: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

// Start begins listening. It returns once the listener is up, so a
// caller that starts the server and then points the broker at it does
// not lose the first report.
//
// A listener that cannot bind is an error: a run that cannot hear its
// fills would log nothing and look healthy.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("fills: listen on %s: %w", s.addr, err)
	}
	s.ln = ln
	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("fill webhook server stopped", "addr", s.addr, "err", err.Error())
		}
	}()
	return nil
}

// Addr returns the address the server listens on.
//
// It is the resolved address, which is the one to point the broker's
// webhook URL at: a server asked to listen on port 0 gets a port the
// operating system chose.
func (s *Server) Addr() string {
	if s.ln == nil {
		return s.addr
	}
	return s.ln.Addr().String()
}

// Stop shuts the server down.
func (s *Server) Stop(ctx context.Context) error {
	if err := s.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("fills: shut down the webhook server: %w", err)
	}
	return nil
}

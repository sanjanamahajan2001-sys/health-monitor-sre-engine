package alert

import (
	"net"
	"net/http"
	"time"
)

type Server struct {
	httpServer *http.Server
	listener   net.Listener
}

func NewServer(addr string, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:           addr,
			Handler:        handler,
			ReadTimeout:    5 * time.Second,
			WriteTimeout:   10 * time.Second,
			IdleTimeout:    30 * time.Second,
			MaxHeaderBytes: 1 << 20,
		},
	}
}

func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	s.listener = ln
	return s.httpServer.Serve(ln)
}

func (s *Server) ListenAndServeTLS(certFile string, keyFile string) error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	s.listener = ln
	return s.httpServer.ServeTLS(ln, certFile, keyFile)
}

func (s *Server) Serve(ln net.Listener) error {
	s.listener = ln
	return s.httpServer.Serve(ln)
}

func (s *Server) ServeTLS(ln net.Listener, certFile string, keyFile string) error {
	s.listener = ln
	return s.httpServer.ServeTLS(ln, certFile, keyFile)
}

func (s *Server) Close() error {
	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

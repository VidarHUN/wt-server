package wt

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"

	invoker "github.com/kixelated/invoker"
	quic "github.com/kixelated/quic-go"
	http3 "github.com/kixelated/quic-go/http3"
	webtransport "github.com/kixelated/webtransport-go"
)

type Server struct {
	inner    *webtransport.Server
	sessions invoker.Tasks
	cert     *tls.Certificate
}

type Config struct {
	Addr string
	Cert *tls.Certificate
}

func New(config Config) (s *Server, err error) {
	s = new(Server)
	s.cert = config.Cert

	quicConfig := &quic.Config{}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*s.cert},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/watch", s.handleWatch)

	s.inner = &webtransport.Server{
		H3: http3.Server{
			TLSConfig:  tlsConfig,
			QuicConfig: quicConfig,
			Addr:       config.Addr,
			Handler:    mux,
		},
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	return s, nil
}

func (s *Server) handleWatch(w http.ResponseWriter, r *http.Request) {
	hijacker, ok := w.(http3.Hijacker)
	if !ok {
		panic("unable to hijack connection: must use kixelated/quic-go")
	}

	conn := hijacker.Connection()

	sess, err := s.inner.Upgrade(w, r)
	if err != nil {
		http.Error(w, "failed to upgrade session", 500)
		return
	}

	err = s.serveSession(r.Context(), conn, sess)
	if err != nil {
		log.Println(err)
	}
}

func (s *Server) serveSession(ctx context.Context, conn quic.Connection, sess *webtransport.Session) (err error) {
	defer func() {
		if err != nil {
			sess.CloseWithError(1, err.Error())
		} else {
			sess.CloseWithError(0, "end of broadcast")
		}
	}()

	ss, err := NewSession(conn, sess)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	err = ss.Run(ctx)
	if err != nil {
		return fmt.Errorf("terminated session: %w", err)
	}

	return nil
}

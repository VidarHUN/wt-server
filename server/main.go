package main

import (
	"crypto/tls"
	"fmt"
	"log"

	"github.com/VidarHUN/wt-server/server/internal/wt"
	"github.com/kixelated/invoker"
)

func main() {
	cert := "../cert/localhost.crt"
	key := "../cert/localhost.key"
	tlsCert, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		fmt.Errorf("failed to load TLS certificate: %w", err)
	}

	wtConfig := wt.Config{
		Addr: "127.0.0.1:4443",
		Cert: &tlsCert,
	}
	wtServer, err := wt.New(wtConfig)
	if err != nil {
		fmt.Errorf("failed to create wt server: %w", err)
	}

	log.Printf("listening on %s", "127.0.0.1:4443")

	invoker.Run(ctx, invoker.Interrupt, wtServer.Run)
}

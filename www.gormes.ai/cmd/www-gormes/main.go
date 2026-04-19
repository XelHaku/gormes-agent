package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/XelHaku/golang-hermes-agent/www.gormes.ai/internal/site"
)

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	flag.Parse()

	handler, err := site.NewServer()
	if err != nil {
		slog.Error("build server", "err", err)
		os.Exit(1)
	}

	slog.Info("www.gormes.ai listening", "addr", *listen)
	if err := http.ListenAndServe(*listen, handler); err != nil {
		slog.Error("serve", "err", err)
		os.Exit(1)
	}
}

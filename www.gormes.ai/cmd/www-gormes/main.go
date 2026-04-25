package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/TrebuchetDynamics/gormes-agent/www.gormes.ai/internal/site"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("www.gormes.ai", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "export" {
		return runExport(args[1:])
	}
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	return runServe(args)
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	listen := fs.String("listen", ":8080", "HTTP listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: www-gormes [serve] [-listen :8080] | export [--out dist]")
	}
	handler, err := site.NewServer()
	if err != nil {
		return err
	}

	slog.Info("www.gormes.ai listening", "addr", *listen)
	return http.ListenAndServe(*listen, handler)
}

func runExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	out := fs.String("out", "dist", "output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: www-gormes export [--out dist]")
	}
	return site.ExportDir(*out)
}

package main

import (
	"context"
	"fmt"

	"github.com/amarbel-llc/lux/internal/config"
	"github.com/amarbel-llc/lux/internal/server"
)

func runLSP(ctx context.Context, lang string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if lang != "" {
		cfg, err = cfg.FilterByLSP(lang)
		if err != nil {
			return fmt.Errorf("filtering config for %s: %w", lang, err)
		}
	}

	srv, err := server.New(cfg, server.WithLSPMode())
	if err != nil {
		return fmt.Errorf("creating LSP server: %w", err)
	}

	return srv.Run(ctx)
}

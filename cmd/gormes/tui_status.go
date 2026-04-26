package main

import (
	"context"
	"os"

	"github.com/TrebuchetDynamics/gormes-agent/internal/doctor"
)

type tuiStartupPreflightOptions struct {
	WorkDir string
}

func runNativeTUIStartupPreflight(_ context.Context, opts tuiStartupPreflightOptions) doctor.CheckResult {
	if opts.WorkDir == "" {
		if wd, err := os.Getwd(); err == nil {
			opts.WorkDir = wd
		}
	}
	return doctorTUIStatus()
}

func doctorTUIStatus() doctor.CheckResult {
	return doctor.CheckResult{
		Name:    "Native TUI",
		Status:  doctor.StatusPass,
		Summary: "available: Go-native Bubble Tea TUI compiled into gormes",
		Items: []doctor.ItemInfo{
			{
				Name:   "runtime",
				Status: doctor.StatusPass,
				Note:   "local startup uses the compiled Go Bubble Tea shell",
			},
			{
				Name:   "offline",
				Status: doctor.StatusPass,
				Note:   "offline mode keeps the same native TUI path without a JavaScript bundle build step",
			},
		},
	}
}

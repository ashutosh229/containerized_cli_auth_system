package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"containerized_cli_auth_system/internal/auth"
	"containerized_cli_auth_system/internal/cli"
	"containerized_cli_auth_system/internal/config"
	"containerized_cli_auth_system/internal/store"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	db, err := store.Open(ctx, cfg.DatabaseDSN, "migrations")
	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	defer db.Close()

	service := auth.NewService(db.Users(), auth.Options{
		SessionTimeout:    cfg.SessionTimeout,
		MaxFailedAttempts: cfg.MaxFailedAttempts,
		LockoutDuration:   cfg.LockoutDuration,
		TOTPIssuer:        cfg.TOTPIssuer,
		BCryptCost:        cfg.BCryptCost,
		Clock:             auth.RealClock{},
	})

	shell := cli.NewShell(os.Stdin, os.Stdout, service)
	if err := shell.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

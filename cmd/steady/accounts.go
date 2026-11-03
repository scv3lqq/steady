package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/term"

	"github.com/scv3lqq/steady/internal/config"
	"github.com/scv3lqq/steady/internal/domain"
	"github.com/scv3lqq/steady/internal/password"
	"github.com/scv3lqq/steady/internal/repository/sqlite"
	"github.com/scv3lqq/steady/internal/usecase"
)

type passwordReader interface {
	ReadPassword(prompt string) ([]byte, error)
}
type terminalPasswordReader struct {
	input  *os.File
	output io.Writer
}

func (r terminalPasswordReader) ReadPassword(prompt string) ([]byte, error) {
	if !term.IsTerminal(int(r.input.Fd())) {
		return nil, fmt.Errorf("password input requires a terminal")
	}
	if _, err := fmt.Fprint(r.output, prompt); err != nil {
		return nil, fmt.Errorf("write password prompt: %w", err)
	}
	value, err := term.ReadPassword(int(r.input.Fd()))
	_, _ = fmt.Fprintln(r.output)
	if err != nil {
		return nil, fmt.Errorf("read password: %w", err)
	}
	return value, nil
}
func runAccount(ctx context.Context, args []string, cfg config.Config, reader passwordReader, output io.Writer) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: steady account <bootstrap|create|reset-password> <username>")
	}
	switch args[0] {
	case "bootstrap", "create", "reset-password":
	default:
		return fmt.Errorf("unknown account command")
	}
	if err := domain.ValidateUsername(args[1]); err != nil {
		return err
	}
	secret, err := reader.ReadPassword("Password: ")
	if err != nil {
		return err
	}
	defer clear(secret)
	if err = domain.ValidatePassword(secret); err != nil {
		return err
	}
	confirm, err := reader.ReadPassword("Confirm password: ")
	if err != nil {
		return err
	}
	defer clear(confirm)
	if string(secret) != string(confirm) {
		return fmt.Errorf("password confirmation does not match")
	}
	store, err := sqlite.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open account store: %w", err)
	}
	defer func() { _ = store.Close() }()
	accounts, err := usecase.NewAccounts(store, password.Hasher{}, time.Now)
	if err != nil {
		return err
	}
	switch args[0] {
	case "bootstrap":
		var created bool
		created, err = accounts.Bootstrap(ctx, args[1], secret)
		if err == nil && !created {
			return writeAccountResult(output, "Bootstrap already complete.")
		}
	case "create":
		err = accounts.Create(ctx, args[1], secret)
	case "reset-password":
		err = accounts.ResetPassword(ctx, args[1], secret)
	}
	if err != nil {
		return err
	}
	return writeAccountResult(output, "Account updated.")
}

func writeAccountResult(output io.Writer, message string) error {
	if _, err := fmt.Fprintln(output, message); err != nil {
		return fmt.Errorf("write account result: %w", err)
	}
	return nil
}

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scv3lqq/steady/internal/config"
	"github.com/scv3lqq/steady/internal/domain"
	"github.com/scv3lqq/steady/internal/password"
	"github.com/scv3lqq/steady/internal/repository/sqlite"
	"github.com/scv3lqq/steady/internal/usecase"
)

type fakePasswordReader struct {
	values  [][]byte
	prompts []string
	err     error
}

func (r *fakePasswordReader) ReadPassword(prompt string) ([]byte, error) {
	r.prompts = append(r.prompts, prompt)
	if r.err != nil {
		return nil, r.err
	}
	if len(r.values) == 0 {
		return nil, errors.New("unexpected prompt")
	}
	v := r.values[0]
	r.values = r.values[1:]
	return v, nil
}
func cliReader() *fakePasswordReader {
	return &fakePasswordReader{values: [][]byte{[]byte("temporary password"), []byte("temporary password")}}
}
func TestAccountCLIRejectsArgumentsBeforePasswordOrDatabase(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown", "alice"}, {"create"}, {"create", "alice", "extra"}, {"create", "alice", "--password=secret"}, {"create", "--password"}, {"create", "Alice"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			r := cliReader()
			cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "missing", "accounts.db")}
			var out bytes.Buffer
			if e := runAccount(t.Context(), args, cfg, r, &out); e == nil {
				t.Fatal("accepted invalid arguments")
			}
			if len(r.prompts) != 0 {
				t.Fatal("prompted before rejecting arguments")
			}
		})
	}
}
func TestAccountCLIMismatchAndReaderErrorDoNotCreateAccount(t *testing.T) {
	for _, r := range []*fakePasswordReader{{values: [][]byte{[]byte("temporary password"), []byte("different password")}}, {err: errors.New("not a terminal")}} {
		cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "accounts.db")}
		var out bytes.Buffer
		if e := runAccount(t.Context(), []string{"create", "alice"}, cfg, r, &out); e == nil {
			t.Fatal("accepted mismatch or nonterminal")
		}
		if strings.Contains(out.String(), "password") {
			t.Fatal("printed credential")
		}
	}
}
func TestAccountCLICreateDuplicateResetBootstrapRetry(t *testing.T) {
	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "accounts.db")}
	var out bytes.Buffer
	r := cliReader()
	if e := runAccount(t.Context(), []string{"bootstrap", "alice"}, cfg, r, &out); e != nil {
		t.Fatal(e)
	}
	if strings.Join(r.prompts, "|") != "Password: |Confirm password: " {
		t.Fatal("hidden password reader prompts")
	}
	if e := runAccount(t.Context(), []string{"bootstrap", "alice"}, cfg, cliReader(), &out); e != nil {
		t.Fatal("same bootstrap retry")
	}
	if e := runAccount(t.Context(), []string{"create", "alice"}, cfg, cliReader(), &out); !errors.Is(e, domain.ErrUsernameExists) {
		t.Fatalf("duplicate: %v", e)
	}
	if e := runAccount(t.Context(), []string{"create", "bobby"}, cfg, cliReader(), &out); e != nil {
		t.Fatal(e)
	}
	if e := runAccount(t.Context(), []string{"reset-password", "alice"}, cfg, &fakePasswordReader{values: [][]byte{[]byte("replacement password"), []byte("replacement password")}}, &out); e != nil {
		t.Fatal(e)
	}
	s, e := sqlite.Open(t.Context(), cfg.DBPath)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = s.Close() }()
	a, e := usecase.NewAccounts(s, password.Hasher{}, time.Now)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = a.Login(t.Context(), "alice", []byte("replacement password")); e != nil {
		t.Fatal("CLI did not persist reset")
	}
	for _, secret := range []string{"temporary password", "replacement password", "argon2", "steady_session"} {
		if strings.Contains(out.String(), secret) {
			t.Fatal("CLI output disclosed credential")
		}
	}
}

func TestTerminalReaderRejectsPipedInputBeforePrompt(t *testing.T) {
	input, writer, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = input.Close(); _ = writer.Close() }()
	var out bytes.Buffer
	r := terminalPasswordReader{input: input, output: &out}
	if _, e = r.ReadPassword("Password: "); e == nil || out.Len() != 0 {
		t.Fatal("piped password input accepted or prompted")
	}
}

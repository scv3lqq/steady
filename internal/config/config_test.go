package config

import (
	"reflect"
	"testing"

	"github.com/rs/zerolog"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr string
	}{
		{
			name: "defaults",
			want: Config{
				Addr:       "127.0.0.1:8770",
				DBPath:     "data/steady.db",
				LogLevel:   zerolog.InfoLevel,
				ExtraHosts: nil,
			},
		},
		{
			name: "overrides",
			env: map[string]string{
				"STEADY_ADDR":      "0.0.0.0:9000",
				"STEADY_DB":        "/tmp/x.db",
				"STEADY_LOG_LEVEL": "DEBUG",
			},
			want: Config{
				Addr:     "0.0.0.0:9000",
				DBPath:   "/tmp/x.db",
				LogLevel: zerolog.DebugLevel,
			},
		},
		{
			// Padding, an empty item and mixed case in one input: it covers trimming,
			// dropping empty items and lowercasing, and fails if any of the three is lost.
			name: "hosts",
			env:  map[string]string{"STEADY_ALLOWED_HOSTS": " a.example , ,B.example:8443 "},
			want: Config{
				Addr:       "127.0.0.1:8770",
				DBPath:     "data/steady.db",
				LogLevel:   zerolog.InfoLevel,
				ExtraHosts: []string{"a.example", "b.example:8443"},
			},
		},
		{
			name:    "bad addr",
			env:     map[string]string{"STEADY_ADDR": "nope"},
			wantErr: `config: STEADY_ADDR "nope": want host:port`,
		},
		{
			name:    "bad port above range",
			env:     map[string]string{"STEADY_ADDR": "127.0.0.1:99999"},
			wantErr: `config: STEADY_ADDR "127.0.0.1:99999": want host:port`,
		},
		{
			name:    "bad port not a number",
			env:     map[string]string{"STEADY_ADDR": "127.0.0.1:x"},
			wantErr: `config: STEADY_ADDR "127.0.0.1:x": want host:port`,
		},
		{
			name:    "bad level",
			env:     map[string]string{"STEADY_LOG_LEVEL": "loud"},
			wantErr: `config: STEADY_LOG_LEVEL "loud": unknown level`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(key string) string { return tc.env[key] }

			got, err := Load(getenv)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("Load() error = nil, want %q", tc.wantErr)
				}
				if err.Error() != tc.wantErr {
					t.Fatalf("Load() error = %q, want %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}
			if got.Addr != tc.want.Addr {
				t.Errorf("Load().Addr = %q, want %q", got.Addr, tc.want.Addr)
			}
			if got.DBPath != tc.want.DBPath {
				t.Errorf("Load().DBPath = %q, want %q", got.DBPath, tc.want.DBPath)
			}
			if got.LogLevel != tc.want.LogLevel {
				t.Errorf("Load().LogLevel = %s, want %s", got.LogLevel, tc.want.LogLevel)
			}
			// reflect.DeepEqual, not slices.Equal: the contract is nil when no host is set,
			// and slices.Equal would accept an empty non-nil slice.
			if !reflect.DeepEqual(got.ExtraHosts, tc.want.ExtraHosts) {
				t.Errorf("Load().ExtraHosts = %#v, want %#v", got.ExtraHosts, tc.want.ExtraHosts)
			}
		})
	}
}

func TestAllowedHosts(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want []string
	}{
		{
			name: "address first, then extra hosts",
			cfg:  Config{Addr: "127.0.0.1:8770", ExtraHosts: []string{"a.example"}},
			want: []string{"127.0.0.1:8770", "a.example"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.AllowedHosts(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("AllowedHosts() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCookieSecureStrictBoolean(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
		bad   bool
	}{{"", true, false}, {"true", true, false}, {"false", false, false}, {"TRUE", false, true}, {"1", false, true}, {" false ", false, true}, {"yes", false, true}} {
		t.Run(tc.value, func(t *testing.T) {
			cfg, e := Load(func(k string) string {
				if k == "STEADY_COOKIE_SECURE" {
					return tc.value
				}
				return ""
			})
			if tc.bad {
				if e == nil {
					t.Fatal("accepted nonliteral boolean")
				}
				return
			}
			if e != nil || cfg.CookieSecure != tc.want {
				t.Fatalf("CookieSecure: %v", e)
			}
		})
	}
}

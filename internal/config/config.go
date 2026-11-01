// Package config reads the STEADY_* environment variables into a validated Config.
package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
)

const (
	defaultAddr   = "127.0.0.1:8770"
	defaultDBPath = "data/steady.db"
)

// Config is the process configuration.
type Config struct {
	Addr       string        // STEADY_ADDR, default "127.0.0.1:8770"
	DBPath     string        // STEADY_DB, default "data/steady.db"
	LogLevel   zerolog.Level // STEADY_LOG_LEVEL, default info
	ExtraHosts []string      // STEADY_ALLOWED_HOSTS, comma-separated, default empty
}

// Load builds a Config from the environment read through getenv (os.Getenv in
// production). An empty value means "use the default".
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:     defaultAddr,
		DBPath:   defaultDBPath,
		LogLevel: zerolog.InfoLevel,
	}

	if v := getenv("STEADY_ADDR"); v != "" {
		if !validAddr(v) {
			return Config{}, fmt.Errorf("config: STEADY_ADDR %q: want host:port", v)
		}
		cfg.Addr = v
	}

	if v := getenv("STEADY_DB"); v != "" {
		cfg.DBPath = v
	}

	if v := getenv("STEADY_LOG_LEVEL"); v != "" {
		level, err := zerolog.ParseLevel(strings.ToLower(v))
		if err != nil || level == zerolog.NoLevel {
			return Config{}, fmt.Errorf("config: STEADY_LOG_LEVEL %q: unknown level", v)
		}
		cfg.LogLevel = level
	}

	cfg.ExtraHosts = parseHosts(getenv("STEADY_ALLOWED_HOSTS"))

	return cfg, nil
}

// AllowedHosts returns the lowercased Addr followed by ExtraHosts, for the HTTP
// layer's Host allowlist.
func (c Config) AllowedHosts() []string {
	return append([]string{strings.ToLower(c.Addr)}, c.ExtraHosts...)
}

// validAddr reports whether addr is host:port with an integer port in 1..65535.
func validAddr(addr string) bool {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

// parseHosts splits a comma-separated host list, trims and lowercases each item
// and drops the empty ones. It returns nil when no item is left.
func parseHosts(v string) []string {
	var hosts []string
	for item := range strings.SplitSeq(v, ",") {
		host := strings.ToLower(strings.TrimSpace(item))
		if host != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

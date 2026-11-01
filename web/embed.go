// Package web holds the frontend, embedded into the binary.
package web

import "embed"

// FS has index.html, styles.css and the compiled dist/*.js at its root.
//
//go:embed index.html styles.css dist/*.js
var FS embed.FS

// Package applog is the CLI's one error log: a plain append-only text file,
// off by default (writes go to io.Discard until Init points it at a file).
// The TUI takes over the whole terminal, so this is the only place a failed
// request's real detail (status, body, which host) survives past the
// scrollback line the user saw.
package applog

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

var (
	mu     sync.Mutex
	logger = log.New(io.Discard, "", log.LstdFlags|log.Lmicroseconds)
	file   *os.File
)

// Init opens path for appending and routes subsequent Errorf calls to it.
// Call once at startup; the returned close func should run on exit (best
// effort — a missed close on a crash loses nothing but the last flush).
func Init(path string) (close func(), err error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return func() {}, err
	}
	mu.Lock()
	file = f
	logger.SetOutput(f)
	mu.Unlock()
	return func() {
		mu.Lock()
		defer mu.Unlock()
		logger.SetOutput(io.Discard)
		_ = f.Close()
		file = nil
	}, nil
}

// Errorf appends one timestamped line. Safe to call before Init (discarded)
// and from multiple goroutines.
func Errorf(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	logger.Output(2, fmt.Sprintf(format, args...)) //nolint:errcheck
}

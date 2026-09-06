package tui

import "github.com/atotto/clipboard"

// clipboardWrite copies s to the system clipboard, ignoring the "no
// clipboard here" error common on headless machines.
func clipboardWrite(s string) error {
	if clipboard.Unsupported {
		return nil
	}
	return clipboard.WriteAll(s)
}

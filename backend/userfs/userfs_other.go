//go:build !windows

// Package userfs runs file operations on the user's own folders with the
// rights of the non-elevated user. Off Windows there is nothing to drop.
package userfs

import "errors"

// ErrDisabled is never returned off Windows.
var ErrDisabled = errors.New("user file access is disabled")

// Enable is a no-op off Windows.
func Enable() error { return nil }

// Disable is a no-op off Windows.
func Disable() {}

// Enabled reports false off Windows.
func Enabled() bool { return false }

// Do runs fn.
func Do(fn func() error) error { return fn() }

//go:build !windows

package main

import (
	"errors"

	"gioui.org/app"
	"gioui.org/io/event"
)

func noteViewEvent(event.Event, *UI) {}

func openDialog(*app.Window, uintptr) (string, bool, error) {
	return "", false, errors.New("file dialog is only wired up on Windows; type a path and press Enter")
}

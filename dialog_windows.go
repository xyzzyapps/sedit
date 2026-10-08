//go:build windows

package main

import (
	"fmt"
	"unsafe"

	"gioui.org/app"
	"gioui.org/io/event"
	"golang.org/x/sys/windows"
)

const (
	ofnExplorer      = 0x00080000
	ofnFileMustExist = 0x00001000
	ofnPathMustExist = 0x00000800
	ofnNoChangeDir   = 0x00000008
)

// openFileName matches OPENFILENAMEW on amd64.
type openFileName struct {
	StructSize      uint32
	Owner           uintptr
	Instance        uintptr
	Filter          *uint16
	CustomFilter    *uint16
	MaxCustomFilter uint32
	FilterIndex     uint32
	File            *uint16
	MaxFile         uint32
	FileTitle       *uint16
	MaxFileTitle    uint32
	InitialDir      *uint16
	Title           *uint16
	Flags           uint32
	FileOffset      uint16
	FileExtension   uint16
	DefExt          *uint16
	CustData        uintptr
	FnHook          uintptr
	TemplateName    *uint16
	PvReserved      uintptr
	DwReserved      uint32
	FlagsEx         uint32
}

var (
	comdlg32             = windows.NewLazySystemDLL("comdlg32.dll")
	procGetOpenFileName  = comdlg32.NewProc("GetOpenFileNameW")
	procCommDlgExtError  = comdlg32.NewProc("CommDlgExtendedError")
)

func noteViewEvent(e event.Event, ui *UI) {
	if v, ok := e.(app.Win32ViewEvent); ok {
		ui.hwnd = v.HWND
	}
}

// openDialog runs the picker on the native window thread and owns it with
// the Gio window, so the dialog stays in front instead of hiding behind a
// frozen frame.
func openDialog(w *app.Window, owner uintptr) (path string, ok bool, err error) {
	w.Run(func() {
		path, ok, err = pickOpenFile(owner)
	})
	return path, ok, err
}

func pickOpenFile(owner uintptr) (string, bool, error) {
	if unsafe.Sizeof(openFileName{}) != 152 {
		return "", false, fmt.Errorf("open dialog: unexpected OPENFILENAMEW size %d", unsafe.Sizeof(openFileName{}))
	}
	file := make([]uint16, 32768)
	filter := utf16Multi("All files", "*.*", "Go", "*.go", "Text", "*.txt;*.md")
	title, err := windows.UTF16PtrFromString("Open")
	if err != nil {
		return "", false, err
	}
	ofn := openFileName{
		Owner:       owner,
		File:        &file[0],
		MaxFile:     uint32(len(file)),
		Filter:      &filter[0],
		FilterIndex: 1,
		Title:       title,
		Flags:       ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnNoChangeDir,
	}
	ofn.StructSize = uint32(unsafe.Sizeof(ofn))
	r, _, callErr := procGetOpenFileName.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		code, _, _ := procCommDlgExtError.Call()
		if code == 0 {
			return "", false, nil
		}
		if callErr != nil && callErr != windows.ERROR_SUCCESS {
			return "", false, fmt.Errorf("open dialog: %w (code %d)", callErr, code)
		}
		return "", false, fmt.Errorf("open dialog failed (code %d)", code)
	}
	return windows.UTF16ToString(file), true, nil
}

func utf16Multi(parts ...string) []uint16 {
	var out []uint16
	for _, p := range parts {
		u, err := windows.UTF16FromString(p)
		if err != nil {
			continue
		}
		out = append(out, u...)
	}
	return append(out, 0)
}

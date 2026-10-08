package main

import "testing"

func TestRoundTripKeepsSourceOrder(t *testing.T) {
	src := "package main\n\nfunc main() {\n\tprintln(1)\n}\n"
	d := newDoc()
	d.Load(src)
	if d.lines[0] != "package main" {
		t.Fatalf("base line = %q", d.lines[0])
	}
	if got := d.Save(); got != src {
		t.Fatalf("save = %q", got)
	}
	// The caret lands on the top of the stack, which is the last source line.
	if d.row != len(d.lines)-1 {
		t.Fatalf("row = %d", d.row)
	}
}

func TestEnterGrowsTowardLaterSourceLine(t *testing.T) {
	d := newDoc()
	d.Load("fmt.Println(1)")
	d.setCaret(0, 0, false)
	d.Enter()
	if len(d.lines) != 2 {
		t.Fatalf("lines = %#v", d.lines)
	}
	if d.lines[0] != "" || d.lines[1] != "fmt.Println(1)" {
		t.Fatalf("lines = %#v", d.lines)
	}
	if d.row != 1 || d.col != 0 {
		t.Fatalf("caret = %d:%d", d.row, d.col)
	}
	// File order is the new line first, then the original line.
	// On screen the original line stays at the bottom and the blank line is above it.
	if d.Save() != "\nfmt.Println(1)" {
		t.Fatalf("save = %q", d.Save())
	}
}

func TestEnterMidLineStacksTailAbove(t *testing.T) {
	d := newDoc()
	d.Load("hello world")
	d.setCaret(0, 5, false)
	d.Enter()
	if d.lines[0] != "hello" || d.lines[1] != " world" {
		t.Fatalf("lines = %#v", d.lines)
	}
}

func TestBackspaceJoinsDownward(t *testing.T) {
	d := newDoc()
	d.Load("base\ntop")
	d.setCaret(1, 0, false)
	d.Backspace()
	if len(d.lines) != 1 || d.lines[0] != "basetop" {
		t.Fatalf("lines = %#v", d.lines)
	}
	if d.col != 4 {
		t.Fatalf("col = %d", d.col)
	}
}

func TestApplyEditUsesSnippetOffsets(t *testing.T) {
	d := newDoc()
	d.Load("ab\ncd")
	d.ApplyEdit(1, 1, "Z")
	if d.Save() != "aZb\ncd" {
		t.Fatalf("save = %q", d.Save())
	}
	// Offset 4 is the 'c' (a Z b \n c).
	d.ApplyEdit(4, 5, "Q")
	if d.Save() != "aZb\nQd" {
		t.Fatalf("save = %q", d.Save())
	}
}

func TestUndoRedo(t *testing.T) {
	d := newDoc()
	d.Insert("ab")
	d.Undo()
	if d.Save() != "" {
		t.Fatalf("undo = %q", d.Save())
	}
	d.Redo()
	if d.Save() != "ab" {
		t.Fatalf("redo = %q", d.Save())
	}
}

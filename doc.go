// Package main is a bottom-up source editor.
//
// The document is stored in normal source order: lines[0] is the first line
// of the file. The view draws that line at the bottom of the window and each
// later line above it, so the text grows toward the top of the screen the way
// a call stack grows.
//
// Loading a file keeps source order. Saving writes source order. A normal
// compiler can read the file. The reversal is only on screen.
package main

import (
	"strings"
	"unicode/utf8"
)

// Doc is the text and the caret. Row 0 is the base of the stack (bottom of
// the view, first line of the file). Col counts runes.
type Doc struct {
	lines []string
	row   int
	col   int

	// anchor is the other end of the selection. When anchorRow < 0 there is
	// no selection.
	anchorRow int
	anchorCol int

	undo []snapshot
	redo []snapshot
}

type snapshot struct {
	lines     []string
	row, col  int
	anchorRow int
	anchorCol int
}

func newDoc() *Doc {
	d := &Doc{lines: []string{""}, anchorRow: -1}
	return d
}

func (d *Doc) cloneSnap() snapshot {
	cp := make([]string, len(d.lines))
	copy(cp, d.lines)
	return snapshot{lines: cp, row: d.row, col: d.col, anchorRow: d.anchorRow, anchorCol: d.anchorCol}
}

func (d *Doc) restore(s snapshot) {
	d.lines = s.lines
	d.row = s.row
	d.col = s.col
	d.anchorRow = s.anchorRow
	d.anchorCol = s.anchorCol
}

func (d *Doc) pushUndo() {
	d.undo = append(d.undo, d.cloneSnap())
	if len(d.undo) > 200 {
		d.undo = d.undo[len(d.undo)-200:]
	}
	d.redo = nil
}

func (d *Doc) Undo() {
	if len(d.undo) == 0 {
		return
	}
	d.redo = append(d.redo, d.cloneSnap())
	s := d.undo[len(d.undo)-1]
	d.undo = d.undo[:len(d.undo)-1]
	d.restore(s)
}

func (d *Doc) Redo() {
	if len(d.redo) == 0 {
		return
	}
	d.undo = append(d.undo, d.cloneSnap())
	s := d.redo[len(d.redo)-1]
	d.redo = d.redo[:len(d.redo)-1]
	d.restore(s)
}

// Load replaces the buffer with the file contents in source order.
// A trailing newline is kept as an empty line at the top of the stack.
func (d *Doc) Load(text string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	d.lines = lines
	d.row = len(lines) - 1
	d.col = runeLen(lines[d.row])
	d.clearSel()
	d.undo = nil
	d.redo = nil
}

// Save returns the file bytes in normal source order, first line first.
func (d *Doc) Save() string {
	return strings.Join(d.lines, "\n")
}

func (d *Doc) clearSel() {
	d.anchorRow = -1
	d.anchorCol = 0
}

func (d *Doc) hasSel() bool {
	return d.anchorRow >= 0 && (d.anchorRow != d.row || d.anchorCol != d.col)
}

func (d *Doc) setCaret(row, col int, extend bool) {
	if row < 0 {
		row = 0
	}
	if row >= len(d.lines) {
		row = len(d.lines) - 1
	}
	n := runeLen(d.lines[row])
	if col < 0 {
		col = 0
	}
	if col > n {
		col = n
	}
	if extend {
		if d.anchorRow < 0 {
			d.anchorRow = d.row
			d.anchorCol = d.col
		}
	} else {
		d.clearSel()
	}
	d.row = row
	d.col = col
}

func (d *Doc) selRange() (r1, c1, r2, c2 int) {
	r1, c1, r2, c2 = d.anchorRow, d.anchorCol, d.row, d.col
	if r1 > r2 || (r1 == r2 && c1 > c2) {
		r1, c1, r2, c2 = r2, c2, r1, c1
	}
	return
}

func (d *Doc) selectedText() string {
	if !d.hasSel() {
		return ""
	}
	r1, c1, r2, c2 := d.selRange()
	if r1 == r2 {
		return runeSlice(d.lines[r1], c1, c2)
	}
	var b strings.Builder
	b.WriteString(runeSlice(d.lines[r1], c1, runeLen(d.lines[r1])))
	b.WriteByte('\n')
	for r := r1 + 1; r < r2; r++ {
		b.WriteString(d.lines[r])
		b.WriteByte('\n')
	}
	b.WriteString(runeSlice(d.lines[r2], 0, c2))
	return b.String()
}

func (d *Doc) deleteSel() {
	if !d.hasSel() {
		return
	}
	r1, c1, r2, c2 := d.selRange()
	left := runeSlice(d.lines[r1], 0, c1)
	right := runeSlice(d.lines[r2], c2, runeLen(d.lines[r2]))
	merged := left + right
	d.lines = append(d.lines[:r1], append([]string{merged}, d.lines[r2+1:]...)...)
	d.row = r1
	d.col = c1
	d.clearSel()
}

// Insert puts text at the caret. Newlines in text each open a line toward
// the top of the stack (a later source line).
func (d *Doc) Insert(text string) {
	if text == "" {
		return
	}
	d.pushUndo()
	d.deleteSel()
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	parts := strings.Split(text, "\n")
	line := d.lines[d.row]
	left := runeSlice(line, 0, d.col)
	right := runeSlice(line, d.col, runeLen(line))
	if len(parts) == 1 {
		d.lines[d.row] = left + parts[0] + right
		d.col += runeLen(parts[0])
		return
	}
	first := left + parts[0]
	last := parts[len(parts)-1] + right
	mid := parts[1 : len(parts)-1]
	var neu []string
	neu = append(neu, d.lines[:d.row]...)
	neu = append(neu, first)
	neu = append(neu, mid...)
	neu = append(neu, last)
	neu = append(neu, d.lines[d.row+1:]...)
	d.lines = neu
	d.row += len(parts) - 1
	d.col = runeLen(parts[len(parts)-1])
}

// Enter splits the line. The text after the caret moves to a new source line
// drawn above this one.
func (d *Doc) Enter() { d.Insert("\n") }

func (d *Doc) Backspace() {
	if d.hasSel() {
		d.pushUndo()
		d.deleteSel()
		return
	}
	if d.col > 0 {
		d.pushUndo()
		line := d.lines[d.row]
		d.lines[d.row] = runeSlice(line, 0, d.col-1) + runeSlice(line, d.col, runeLen(line))
		d.col--
		return
	}
	if d.row == 0 {
		return
	}
	d.pushUndo()
	prev := d.lines[d.row-1]
	d.col = runeLen(prev)
	d.lines[d.row-1] = prev + d.lines[d.row]
	d.lines = append(d.lines[:d.row], d.lines[d.row+1:]...)
	d.row--
}

func (d *Doc) DeleteForward() {
	if d.hasSel() {
		d.pushUndo()
		d.deleteSel()
		return
	}
	line := d.lines[d.row]
	if d.col < runeLen(line) {
		d.pushUndo()
		d.lines[d.row] = runeSlice(line, 0, d.col) + runeSlice(line, d.col+1, runeLen(line))
		return
	}
	if d.row >= len(d.lines)-1 {
		return
	}
	d.pushUndo()
	d.lines[d.row] = line + d.lines[d.row+1]
	d.lines = append(d.lines[:d.row+1], d.lines[d.row+2:]...)
}

// runeIndex maps a caret to an offset in the snippet, counting a newline
// between source lines.
func (d *Doc) runeIndex(row, col int) int {
	if row < 0 {
		row = 0
	}
	if row >= len(d.lines) {
		row = len(d.lines) - 1
	}
	n := 0
	for i := 0; i < row; i++ {
		n += runeLen(d.lines[i]) + 1
	}
	if col < 0 {
		col = 0
	}
	if col > runeLen(d.lines[row]) {
		col = runeLen(d.lines[row])
	}
	return n + col
}

func (d *Doc) posAt(index int) (row, col int) {
	if index < 0 {
		index = 0
	}
	for row = 0; row < len(d.lines); row++ {
		n := runeLen(d.lines[row])
		if index <= n {
			return row, index
		}
		index -= n + 1
	}
	last := len(d.lines) - 1
	return last, runeLen(d.lines[last])
}

// snippet is the whole buffer in source order, plus the selection as rune offsets.
func (d *Doc) snippet() (text string, start, end int) {
	text = d.Save()
	if d.hasSel() {
		r1, c1, r2, c2 := d.selRange()
		return text, d.runeIndex(r1, c1), d.runeIndex(r2, c2)
	}
	i := d.runeIndex(d.row, d.col)
	return text, i, i
}

// SetRange moves the selection to snippet rune offsets.
func (d *Doc) SetRange(start, end int) {
	if start > end {
		start, end = end, start
	}
	r1, c1 := d.posAt(start)
	r2, c2 := d.posAt(end)
	d.anchorRow, d.anchorCol = r1, c1
	d.row, d.col = r2, c2
	if d.anchorRow == d.row && d.anchorCol == d.col {
		d.clearSel()
	}
}

// ApplyEdit replaces a snippet rune range. Typed characters arrive this way.
func (d *Doc) ApplyEdit(start, end int, text string) {
	if start > end {
		start, end = end, start
	}
	r1, c1 := d.posAt(start)
	r2, c2 := d.posAt(end)
	if r1 == r2 && c1 == c2 {
		d.setCaret(r1, c1, false)
	} else {
		d.anchorRow, d.anchorCol = r1, c1
		d.row, d.col = r2, c2
	}
	d.Insert(text)
}

func (d *Doc) SelectAll() {
	if len(d.lines) == 0 {
		return
	}
	d.anchorRow = 0
	d.anchorCol = 0
	d.row = len(d.lines) - 1
	d.col = runeLen(d.lines[d.row])
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func runeSlice(s string, from, to int) string {
	if from < 0 {
		from = 0
	}
	r := []rune(s)
	if to > len(r) {
		to = len(r)
	}
	if from > to {
		from = to
	}
	return string(r[from:to])
}

package main

import (
	"image"
	"image/color"
	"io"
	"log"
	"os"
	"strings"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type editorTag struct{}

type UI struct {
	th   *material.Theme
	doc  *Doc
	path widget.Editor

	openClick   widget.Clickable
	saveClick   widget.Clickable
	pendingOpen bool
	hwnd        uintptr

	file        string
	status      string
	dirty       bool
	scroll      int
	bar         widget.Scrollbar
	seenRow     int
	seenCol     int
	editorFocus bool
	refocus     bool

	linePx   int
	gutterPx int
	charPx   int
	body     image.Point
}

func main() {
	ui := newUI()
	if len(os.Args) > 1 {
		ui.open(os.Args[1])
	}
	go func() {
		w := new(app.Window)
		w.Option(app.Title("sedit"), app.Size(unit.Dp(960), unit.Dp(720)))
		if err := ui.loop(w); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func newUI() *UI {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	ui := &UI{
		th:     th,
		doc:    newDoc(),
		status:      "view grows upward · the file on disk stays in normal source order",
		editorFocus: true,
		refocus:     true,
		seenRow:     -1,
	}
	ui.path.SingleLine = true
	ui.path.Submit = true
	return ui
}

func (ui *UI) noteView(e event.Event) { noteViewEvent(e, ui) }

func (ui *UI) loop(w *app.Window) error {
	var ops op.Ops
	for {
		e := w.Event()
		ui.noteView(e)
		switch e := e.(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			ui.frame(gtx)
			e.Frame(gtx.Ops)
			if ui.pendingOpen {
				ui.pendingOpen = false
				path, ok, err := openDialog(w, ui.hwnd)
				switch {
				case err != nil:
					ui.status = err.Error()
				case ok:
					ui.open(path)
				default:
					ui.status = "open canceled"
				}
				w.Invalidate()
			}
		}
	}
}

func (ui *UI) frame(gtx layout.Context) layout.Dimensions {
	paint.Fill(gtx.Ops, colBg)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(ui.layoutBar),
		layout.Flexed(1, ui.layoutBody),
		layout.Rigid(ui.layoutStatus),
	)
}

func (ui *UI) layoutBar(gtx layout.Context) layout.Dimensions {
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if ui.openClick.Clicked(gtx) {
			ui.pendingOpen = true
		}
		if ui.saveClick.Clicked(gtx) {
			ui.save()
		}
		for {
			ev, ok := ui.path.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				ui.open(ui.path.Text())
			}
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				b := material.Button(ui.th, &ui.openClick, "Open")
				b.TextSize = unit.Sp(13)
				b.Background = colBtn
				b.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}
				return b.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				b := material.Button(ui.th, &ui.saveClick, "Save")
				b.TextSize = unit.Sp(13)
				b.Background = colBtn
				b.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(12), Right: unit.Dp(12)}
				return b.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						paint.FillShape(gtx.Ops, colLine, clip.Rect{Max: gtx.Constraints.Min}.Op())
						return layout.Dimensions{Size: gtx.Constraints.Min}
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						ed := material.Editor(ui.th, &ui.path, "path, or use Open")
						ed.Font = mono
						ed.TextSize = unit.Sp(14)
						ed.Color = colText
						ed.HintColor = colDim
						gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(28))
						return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8), Top: unit.Dp(6), Bottom: unit.Dp(6)}.Layout(gtx, ed.Layout)
					}),
				)
			}),
		)
	})
}

func (ui *UI) layoutStatus(gtx layout.Context) layout.Dimensions {
	line := ui.doc.row + 1
	n := len(ui.doc.lines)
	mark := ""
	if ui.dirty {
		mark = "  ·  modified"
	}
	msg := ui.status + mark
	meta := ""
	if ui.file != "" {
		meta = ui.file + "    "
	}
	meta += "stack " + itoa(line) + "/" + itoa(n) + "   col " + itoa(ui.doc.col+1)
	return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(4), Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return ui.label(gtx, msg, colDim, unit.Sp(12))
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.label(gtx, meta, colDim, unit.Sp(12))
			}),
		)
	})
}

func (ui *UI) layoutBody(gtx layout.Context) layout.Dimensions {
	ui.linePx = gtx.Sp(unit.Sp(20))
	ui.gutterPx = gtx.Dp(unit.Dp(64))
	ui.charPx = ui.measure(gtx, "M")
	if ui.charPx < 1 {
		ui.charPx = gtx.Sp(unit.Sp(9))
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, ui.layoutText),
		layout.Rigid(ui.layoutScrollbar),
	)
}

func (ui *UI) layoutText(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	ui.body = size

	// Clip and register input in body coordinates.
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, editorTag{})
	ui.handleKeys(gtx)
	ui.handlePointer(gtx)

	// Keep the caret on screen. scroll shifts the stack downward.
	ui.reveal()

	stack := clip.Rect{Max: size}.Push(gtx.Ops)
	caret := ui.paintLines(gtx)
	stack.Pop()
	ui.publishCaret(gtx, caret)

	if ui.refocus {
		gtx.Execute(key.FocusCmd{Tag: editorTag{}})
		ui.refocus = false
	}
	return layout.Dimensions{Size: size}
}

func (ui *UI) layoutScrollbar(gtx layout.Context) layout.Dimensions {
	width := gtx.Dp(unit.Dp(12))
	height := gtx.Constraints.Max.Y
	if height < 1 {
		height = 1
	}
	gtx.Constraints.Min = image.Pt(width, height)
	gtx.Constraints.Max = gtx.Constraints.Min

	start, end, content := ui.viewport()
	// A full viewport is not scrollable. Keep the column so the text
	// width does not jump when the file grows past the window.
	if end-start >= 1 {
		paint.FillShape(gtx.Ops, colLine, clip.Rect{Max: gtx.Constraints.Max}.Op())
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}

	st := material.Scrollbar(ui.th, &ui.bar)
	st.Track.Color = colLine
	st.Indicator.MinorWidth = unit.Dp(8)
	st.Indicator.CornerRadius = unit.Dp(4)
	st.Indicator.Color = color.NRGBA{R: 0x7a, G: 0xa2, B: 0xf7, A: 0xcc}
	st.Indicator.HoverColor = colText
	dims := st.Layout(gtx, layout.Vertical, start, end)
	if d := ui.bar.ScrollDistance(); d != 0 {
		ui.scroll -= int(d * content)
		ui.clampScroll()
	}
	return dims
}

// viewport reports the visible fraction of the stack in normal scrollbar
// space: 0 is the top of the window, 1 is the bottom. scroll 0 shows the
// base of the stack, so the thumb rests at the bottom.
func (ui *UI) viewport() (start, end, content float32) {
	content = float32(len(ui.doc.lines) * ui.linePx)
	view := float32(ui.body.Y)
	if content < 1 {
		content = 1
	}
	if view < 1 {
		view = 1
	}
	if content <= view {
		return 0, 1, content
	}
	start = (content - view - float32(ui.scroll)) / content
	end = (content - float32(ui.scroll)) / content
	if start < 0 {
		start = 0
	}
	if end > 1 {
		end = 1
	}
	if end < start {
		end = start
	}
	return start, end, content
}

func (ui *UI) handleKeys(gtx layout.Context) {
	filters := []event.Filter{
		key.FocusFilter{Target: editorTag{}},
		key.Filter{Focus: editorTag{}, Name: key.NameTab, Optional: key.ModShift},
		key.Filter{Focus: editorTag{}, Name: key.NameReturn, Optional: key.ModShift},
		key.Filter{Focus: editorTag{}, Name: key.NameEnter},
		key.Filter{Focus: editorTag{}, Name: key.NameDeleteBackward, Optional: key.ModShortcut | key.ModShift},
		key.Filter{Focus: editorTag{}, Name: key.NameDeleteForward, Optional: key.ModShortcut | key.ModShift},
		key.Filter{Focus: editorTag{}, Name: key.NameUpArrow, Optional: key.ModShortcut | key.ModShift},
		key.Filter{Focus: editorTag{}, Name: key.NameDownArrow, Optional: key.ModShortcut | key.ModShift},
		key.Filter{Focus: editorTag{}, Name: key.NameLeftArrow, Optional: key.ModShortcut | key.ModShift},
		key.Filter{Focus: editorTag{}, Name: key.NameRightArrow, Optional: key.ModShortcut | key.ModShift},
		key.Filter{Focus: editorTag{}, Name: key.NameHome, Optional: key.ModShortcut | key.ModShift},
		key.Filter{Focus: editorTag{}, Name: key.NameEnd, Optional: key.ModShortcut | key.ModShift},
		key.Filter{Focus: editorTag{}, Name: key.NamePageUp, Optional: key.ModShift},
		key.Filter{Focus: editorTag{}, Name: key.NamePageDown, Optional: key.ModShift},
		key.Filter{Focus: editorTag{}, Name: "A", Required: key.ModShortcut},
		key.Filter{Focus: editorTag{}, Name: "C", Required: key.ModShortcut},
		key.Filter{Focus: editorTag{}, Name: "X", Required: key.ModShortcut},
		key.Filter{Focus: editorTag{}, Name: "V", Required: key.ModShortcut},
		key.Filter{Focus: editorTag{}, Name: "S", Required: key.ModShortcut},
		key.Filter{Focus: editorTag{}, Name: "Z", Required: key.ModShortcut, Optional: key.ModShift},
		key.Filter{Focus: editorTag{}, Name: "Y", Required: key.ModShortcut},
		key.Filter{Focus: editorTag{}, Optional: key.ModShift | key.ModShortcut},
		transfer.TargetFilter{Target: editorTag{}, Type: "application/text"},
	}
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		switch ev := ev.(type) {
		case key.FocusEvent:
			ui.editorFocus = ev.Focus
		case key.EditEvent:
			ui.doc.ApplyEdit(ev.Range.Start, ev.Range.End, ev.Text)
			ui.dirty = true
		case key.SelectionEvent:
			ui.doc.SetRange(int(ev.Start), int(ev.End))
		case key.Event:
			if ev.State != key.Press {
				continue
			}
			ui.onKey(gtx, ev)
		case transfer.DataEvent:
			rc := ev.Open()
			b, err := io.ReadAll(rc)
			rc.Close()
			if err == nil && len(b) > 0 {
				ui.doc.Insert(string(b))
				ui.dirty = true
				ui.status = "pasted"
			}
		}
	}
}

func (ui *UI) onKey(gtx layout.Context, ev key.Event) {
	d := ui.doc
	shift := ev.Modifiers.Contain(key.ModShift)
	shortcut := ev.Modifiers.Contain(key.ModShortcut)
	switch ev.Name {
	case "S":
		if shortcut {
			ui.save()
		}
	case "Z":
		if shortcut && shift {
			d.Redo()
		} else if shortcut {
			d.Undo()
		}
		ui.dirty = true
	case "Y":
		if shortcut {
			d.Redo()
			ui.dirty = true
		}
	case "A":
		if shortcut {
			d.SelectAll()
		}
	case "C":
		if shortcut {
			ui.copy(gtx)
		}
	case "X":
		if shortcut {
			ui.cut(gtx)
		}
	case "V":
		if shortcut {
			gtx.Execute(clipboard.ReadCmd{Tag: editorTag{}})
		}
	case key.NameReturn, key.NameEnter:
		d.Enter()
		ui.dirty = true
		ui.status = "new line above the caret"
	case key.NameTab:
		d.Insert("    ")
		ui.dirty = true
	case key.NameDeleteBackward:
		d.Backspace()
		ui.dirty = true
	case key.NameDeleteForward:
		d.DeleteForward()
		ui.dirty = true
	case key.NameUpArrow:
		// Visual up is a later source line, toward the top of the stack.
		if shortcut {
			d.setCaret(len(d.lines)-1, d.col, shift)
		} else {
			d.setCaret(d.row+1, d.col, shift)
		}
	case key.NameDownArrow:
		if shortcut {
			d.setCaret(0, d.col, shift)
		} else {
			d.setCaret(d.row-1, d.col, shift)
		}
	case key.NameLeftArrow:
		if d.col == 0 && d.row > 0 {
			d.setCaret(d.row-1, runeLen(d.lines[d.row-1]), shift)
		} else {
			d.setCaret(d.row, d.col-1, shift)
		}
	case key.NameRightArrow:
		if d.col >= runeLen(d.lines[d.row]) && d.row < len(d.lines)-1 {
			d.setCaret(d.row+1, 0, shift)
		} else {
			d.setCaret(d.row, d.col+1, shift)
		}
	case key.NameHome:
		if shortcut {
			d.setCaret(0, 0, shift)
		} else {
			d.setCaret(d.row, 0, shift)
		}
	case key.NameEnd:
		if shortcut {
			last := len(d.lines) - 1
			d.setCaret(last, runeLen(d.lines[last]), shift)
		} else {
			d.setCaret(d.row, runeLen(d.lines[d.row]), shift)
		}
	case key.NamePageUp:
		page := ui.body.Y / ui.linePx
		if page < 1 {
			page = 1
		}
		d.setCaret(d.row+page, d.col, shift)
	case key.NamePageDown:
		page := ui.body.Y / ui.linePx
		if page < 1 {
			page = 1
		}
		d.setCaret(d.row-page, d.col, shift)
	}
}

func (ui *UI) copy(gtx layout.Context) {
	text := ui.doc.selectedText()
	if text == "" {
		text = ui.doc.lines[ui.doc.row] + "\n"
	}
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
	ui.status = "copied in source order"
}

func (ui *UI) cut(gtx layout.Context) {
	text := ui.doc.selectedText()
	if text == "" {
		return
	}
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
	ui.doc.pushUndo()
	ui.doc.deleteSel()
	ui.dirty = true
	ui.status = "cut"
}

func (ui *UI) handlePointer(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  editorTag{},
			Kinds:   pointer.Press | pointer.Scroll | pointer.Drag,
			ScrollY: pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Scroll:
			ui.scroll += int(e.Scroll.Y)
			ui.clampScroll()
		case pointer.Press, pointer.Drag:
			row, col := ui.posToCaretF(e.Position.Y, e.Position.X)
			extend := e.Modifiers.Contain(key.ModShift) || e.Kind == pointer.Drag
			ui.doc.setCaret(row, col, extend)
			ui.editorFocus = true
			ui.refocus = true
		}
	}
}

func (ui *UI) paintLines(gtx layout.Context) image.Point {
	d := ui.doc
	h := ui.body.Y
	for i := range d.lines {
		top := ui.lineTop(i)
		if top+ui.linePx < 0 || top > h {
			continue
		}
		if i == d.row {
			r := clip.Rect{Min: image.Pt(0, top), Max: image.Pt(ui.body.X, top+ui.linePx)}
			paint.FillShape(gtx.Ops, colLine, r.Op())
		}
		num := itoa(i + 1)
		ui.paintText(gtx, num, ui.gutterPx-ui.charPx*len(num)-gtx.Dp(unit.Dp(8)), top, colDim)
		shown := strings.ReplaceAll(d.lines[i], "\t", "    ")
		if d.hasSel() {
			ui.paintSelection(gtx, i, top)
		}
		ui.paintText(gtx, shown, ui.gutterPx, top, colText)
		if i == 0 {
			// Mark the base of the stack.
			bar := clip.Rect{Min: image.Pt(0, top), Max: image.Pt(gtx.Dp(unit.Dp(3)), top+ui.linePx)}
			paint.FillShape(gtx.Ops, colBase, bar.Op())
		}
	}
	// Caret. Its point is also reported to the input method.
	top := ui.lineTop(d.row)
	prefix := runeSlice(strings.ReplaceAll(d.lines[d.row], "\t", "    "), 0, d.col)
	x := ui.gutterPx + ui.measure(gtx, prefix)
	if ui.editorFocus {
		caret := clip.Rect{Min: image.Pt(x, top+2), Max: image.Pt(x+gtx.Dp(unit.Dp(2)), top+ui.linePx-2)}
		paint.FillShape(gtx.Ops, colCaret, caret.Op())
	}
	return image.Pt(x, top+ui.linePx)
}

func (ui *UI) paintSelection(gtx layout.Context, i, top int) {
	r1, c1, r2, c2 := ui.doc.selRange()
	if i < r1 || i > r2 {
		return
	}
	line := strings.ReplaceAll(ui.doc.lines[i], "\t", "    ")
	start, end := 0, runeLen(line)
	if i == r1 {
		start = c1
	}
	if i == r2 {
		end = c2
	}
	if start > end {
		return
	}
	x0 := ui.gutterPx + ui.measure(gtx, runeSlice(line, 0, start))
	x1 := ui.gutterPx + ui.measure(gtx, runeSlice(line, 0, end))
	if x1 < x0+2 && start == end {
		return
	}
	if i != r2 && end == runeLen(line) {
		x1 = ui.body.X
	}
	r := clip.Rect{Min: image.Pt(x0, top), Max: image.Pt(x1, top+ui.linePx)}
	paint.FillShape(gtx.Ops, colSel, r.Op())
}

func (ui *UI) lineTop(i int) int {
	// Source line 0 sits on the bottom. Later lines stack above it.
	// scroll > 0 moves the stack down so upper lines fit in the window.
	return ui.body.Y - (i+1)*ui.linePx + ui.scroll
}

func (ui *UI) reveal() {
	if ui.linePx <= 0 {
		return
	}
	// Scrolling the bar or the wheel must not drag the view back to the caret.
	// Follow the caret only when it actually moves.
	if ui.seenRow == ui.doc.row && ui.seenCol == ui.doc.col {
		return
	}
	ui.seenRow = ui.doc.row
	ui.seenCol = ui.doc.col
	top := ui.lineTop(ui.doc.row)
	if top < 0 {
		ui.scroll -= top
	}
	if top+ui.linePx > ui.body.Y {
		ui.scroll -= top + ui.linePx - ui.body.Y
	}
	ui.clampScroll()
}

func (ui *UI) clampScroll() {
	content := len(ui.doc.lines) * ui.linePx
	max := content - ui.body.Y
	if max < 0 {
		max = 0
	}
	if ui.scroll < 0 {
		ui.scroll = 0
	}
	if ui.scroll > max {
		ui.scroll = max
	}
}

func (ui *UI) posToCaretF(y, x float32) (int, int) {
	if ui.linePx <= 0 {
		return 0, 0
	}
	// Invert lineTop: top = bodyY - (i+1)*lh + scroll
	// i = (bodyY + scroll - top) / lh - 1
	top := int(y)
	i := (ui.body.Y + ui.scroll - top) / ui.linePx
	// lineTop(i) is the top; the integer division above lands on i when
	// top is inside the line only if we account for the remainder.
	// top = body - (i+1)*lh + scroll  => (i+1)*lh = body + scroll - top => i+1 = ceilDiv
	span := ui.body.Y + ui.scroll - top
	if span <= 0 {
		i = -1
	} else {
		i = (span - 1) / ui.linePx
	}
	if i < 0 {
		i = 0
	}
	if i >= len(ui.doc.lines) {
		i = len(ui.doc.lines) - 1
	}
	rel := int(x) - ui.gutterPx
	if rel <= 0 {
		return i, 0
	}
	line := strings.ReplaceAll(ui.doc.lines[i], "\t", "    ")
	prev := 0
	col := 0
	for range line {
		col++
		width := col * ui.charPx
		if rel < (prev+width)/2 {
			return i, col - 1
		}
		prev = width
	}
	return i, col
}

func (ui *UI) measure(gtx layout.Context, s string) int {
	if s == "" {
		return 0
	}
	gtx.Constraints.Min = image.Point{}
	gtx.Constraints.Max = image.Pt(1e6, 1e6)
	l := material.Label(ui.th, unit.Sp(16), s)
	l.Font = mono
	l.Color = colText
	// Record so the measurement does not paint.
	macro := op.Record(gtx.Ops)
	dims := l.Layout(gtx)
	macro.Stop()
	return dims.Size.X
}

func (ui *UI) paintText(gtx layout.Context, s string, x, y int, c color.NRGBA) {
	if s == "" {
		return
	}
	gtx2 := gtx
	gtx2.Constraints.Min = image.Point{}
	gtx2.Constraints.Max = image.Pt(1e6, ui.linePx)
	macro := op.Record(gtx.Ops)
	l := material.Label(ui.th, unit.Sp(16), s)
	l.Font = mono
	l.Color = c
	l.MaxLines = 1
	dims := l.Layout(gtx2)
	call := macro.Stop()
	// Vertically center the glyph box in the line slot.
	dy := y
	if dims.Size.Y < ui.linePx {
		dy = y + (ui.linePx-dims.Size.Y)/2
	}
	stack := op.Offset(image.Pt(x, dy)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	stack.Pop()
}

// publishCaret tells the window where the caret is and which text it edits.
// Windows delivers typed characters as edits against that snippet. Without it,
// keystrokes never reach this buffer.
func (ui *UI) publishCaret(gtx layout.Context, caret image.Point) {
	d := ui.doc
	text, a, b := d.snippet()
	gtx.Execute(key.SnippetCmd{
		Tag: editorTag{},
		Snippet: key.Snippet{
			Range: key.Range{Start: 0, End: runeLen(text)},
			Text:  text,
		},
	})
	gtx.Execute(key.SelectionCmd{
		Tag:   editorTag{},
		Range: key.Range{Start: a, End: b},
		Caret: key.Caret{
			Pos:     layout.FPt(caret),
			Ascent:  float32(ui.linePx),
			Descent: 2,
		},
	})
}

func (ui *UI) label(gtx layout.Context, s string, c color.NRGBA, size unit.Sp) layout.Dimensions {
	l := material.Label(ui.th, size, s)
	l.Color = c
	l.Font = mono
	l.MaxLines = 1
	return l.Layout(gtx)
}

func (ui *UI) open(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		ui.status = "type a path, then Open"
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		ui.status = err.Error()
		return
	}
	ui.doc.Load(string(b))
	ui.file = path
	ui.path.SetText(path)
	ui.dirty = false
	ui.scroll = 0
	ui.seenRow = -1
	ui.refocus = true
	ui.status = "opened · first source line is the bottom of the stack"
}

func (ui *UI) save() {
	path := strings.TrimSpace(ui.path.Text())
	if path == "" {
		path = ui.file
	}
	if path == "" {
		ui.status = "type a path, then Save"
		return
	}
	text := ui.doc.Save()
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil && dirOf(path) != "." {
		ui.status = err.Error()
		return
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		ui.status = err.Error()
		return
	}
	ui.file = path
	ui.dirty = false
	ui.status = "saved in source order (top of stack is the last line of the file)"
}

func dirOf(path string) string {
	d := strings.ReplaceAll(path, "\\", "/")
	i := strings.LastIndex(d, "/")
	if i <= 0 {
		return "."
	}
	return path[:i]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

var (
	colBg    = color.NRGBA{R: 0x1a, G: 0x1b, B: 0x26, A: 0xff}
	colLine  = color.NRGBA{R: 0x24, G: 0x28, B: 0x3b, A: 0xff}
	colText  = color.NRGBA{R: 0xc0, G: 0xca, B: 0xf5, A: 0xff}
	colDim   = color.NRGBA{R: 0x56, G: 0x5f, B: 0x89, A: 0xff}
	colCaret = color.NRGBA{R: 0x7a, G: 0xa2, B: 0xf7, A: 0xff}
	colBtn   = color.NRGBA{R: 0x3d, G: 0x59, B: 0xa1, A: 0xff}
	colSel   = color.NRGBA{R: 0x2a, G: 0x3f, B: 0x6e, A: 0xff}
	colBase  = color.NRGBA{R: 0x9e, G: 0xce, B: 0x6a, A: 0xff}
	mono     = font.Font{Typeface: "Go Mono"}
)



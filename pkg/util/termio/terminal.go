// Copyright Consensys Incorporated
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
// an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
// specific language governing permissions and limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0
package termio

import (
	"errors"
	"io"
	"math"
	"os"
	"slices"
	"sync"

	"golang.org/x/term"
)

// ESC is the escape code.
const ESC uint16 = 0x1b

// TAB indicates the horizontal tab
const TAB uint16 = 0x09

// CARRIAGE_RETURN indicates "enter"
const CARRIAGE_RETURN uint16 = 0x0D

// BACKSPACE is the backspace
const BACKSPACE uint16 = 0x08

// DEL is the delete key
const DEL uint16 = 0x7f

// BACKTAB indicates shift + tab
const BACKTAB uint16 = 0x5b5a

// CURSOR_UP (up arrow)
const CURSOR_UP uint16 = 0x5b41

// CURSOR_DOWN (down arrow)
const CURSOR_DOWN uint16 = 0x5b42

// CURSOR_LEFT (left arrow)
const CURSOR_LEFT uint16 = 0x5b43

// CURSOR_RIGHT (left arrow)
const CURSOR_RIGHT uint16 = 0x5b44

// SCROLL_UP (page up)
const SCROLL_UP uint16 = 0x5b53

// SCROLL_DOWN (page down)
const SCROLL_DOWN uint16 = 0x5b54

// UNKNOWN is a fall-back for unknown escape sequences
const UNKNOWN uint16 = 0x5bff

// Terminal provides a simple top-level window.
type Terminal struct {
	// file descriptor for output.
	fd int
	// Underlying terminal
	xterm *term.Terminal
	// Input buffer to handling escapes, etc.
	input terminalInput
	// Stores original state of terminal so this can be restored.
	state *term.State
	// List of widgets to display
	widgets []Widget
}

// NewTerminal constructs a new terminal.
func NewTerminal() (*Terminal, error) {
	fd := int(os.Stdout.Fd())
	//
	if !term.IsTerminal(fd) {
		return nil, errors.New("invalid terminal")
	}
	// Move terminal into raw mode
	state, err := term.MakeRaw(0)
	if err != nil {
		return nil, err
	}
	// Construct "screen"
	screen := struct {
		io.Reader
		io.Writer
	}{os.Stdin, os.Stdout}
	// Grab terminal screen
	terminal := term.NewTerminal(screen, "")
	input := newTerminalInput()
	//
	return &Terminal{fd, terminal, input, state, nil}, nil
}

// ReadKey returns a keyevent from the keyboard.  This is either an ASCII
// character, or an extended escape code.
func (t *Terminal) ReadKey() (uint16, error) {
	return t.input.Read()
}

// GetSize returns the dimensions of the terminal.
func (t *Terminal) GetSize() (uint, uint) {
	w, h, err := term.GetSize(t.fd)
	// Sanity check for now
	if err != nil {
		panic(err)
	}
	//
	return uint(w), uint(h)
}

// Add a new widget to this window.  Widgets will be laid out vertically in
// the order they are added.
func (t *Terminal) Add(w Widget) {
	t.widgets = append(t.widgets, w)
}

// Set the widget at a specific position, overwriting what was there before.
func (t *Terminal) Set(i uint, w Widget) {
	t.widgets[i] = w
}

// Lock on writing
var renderLock sync.Mutex

// Render this window to the terminal.
func (t *Terminal) Render() error {
	var (
		taken uint
		nFlex uint
		flex  uint
	)
	// Determine dimensions
	width, height := t.GetSize()
	//
	for _, w := range t.widgets {
		if h := w.GetHeight(); h != math.MaxUint {
			taken += h
		} else {
			nFlex++
		}
	}
	// Determine flexible amount
	if nFlex > 0 {
		flex = (height - taken) / nFlex
	}
	// Reset taken
	taken = 0
	// Issue CUP command to reset cursor to top-left position.  This helps to
	// avoid screen tearing.  We could go even better by only redrawing those
	// lines (or sections of lines) which had actually changed.  However, that
	// seems to be going overboard.
	buffer := []byte{0x1b, '[', 'H'}
	//
	for _, w := range t.widgets {
		var h uint
		//
		if h = w.GetHeight(); h == math.MaxUint {
			h = flex
		}
		// Construct canvas
		canvas := newTerminalCanvas(width, h)
		// Record how much taken
		taken += h
		// Render widget
		w.Render(canvas)
		// Render canvas
		buffer = t.renderCanvas(buffer, canvas)
	}
	// Check whether any left
	if taken < height {
		blank := blankLine(width)
		// Fill out remainder with blanks
		for ; taken < height; taken++ {
			buffer = append(buffer, blank...)
		}
	}
	//
	renderLock.Lock()
	//
	for {
		// Write as much as we can.
		n, err := t.xterm.Write(buffer)
		// Check what happened
		if err != nil {
			renderLock.Unlock()
			return err
		} else if n == len(buffer) {
			renderLock.Unlock()
			return nil
		}
		//
		buffer = buffer[n:]
	}
}

// Restore terminal to its original state.
func (t *Terminal) Restore() error {
	return term.Restore(t.fd, t.state)
}

func (t *Terminal) renderCanvas(buffer []byte, canvas *terminalCanvas) []byte {
	for i := uint(0); i < uint(len(canvas.lines)); i++ {
		line := canvas.renderLine(i)
		// Render the line
		buffer = append(buffer, line...)
	}
	//
	return buffer
}

// TerminalCanvas provides a Canvas which collects chunks to be written when
// rendering a given widget.
type terminalCanvas struct {
	width uint
	lines [][]terminalChunk
}

func newTerminalCanvas(width, height uint) *terminalCanvas {
	return &terminalCanvas{width, make([][]terminalChunk, height)}
}

func (p *terminalCanvas) GetDimensions() (uint, uint) {
	return p.width, uint(len(p.lines))
}

func (p *terminalCanvas) Write(x, y uint, text FormattedText) {
	// Determine dimensions
	w, h := p.GetDimensions()
	// Clip chunk if necessary
	if x < w && y < h {
		mx := x + text.Len()
		if mx > w {
			text = text.Clip(0, w-x)
		}
		//
		p.lines[y] = append(p.lines[y], terminalChunk{x, text})
	}
}

func (p *terminalCanvas) renderLine(line uint) []byte {
	var (
		xpos   uint   = 0
		chunks        = p.lines[line]
		bytes  []byte = nil
	)
	// Sort by decreasing x value.
	slices.SortFunc(chunks, func(l terminalChunk, r terminalChunk) int {
		if l.xpos < r.xpos {
			return -1
		} else if l.xpos > r.xpos {
			return 1
		}
		//
		return 0
	})
	// Process each chunk in turn
	for _, c := range chunks {
		c_text := c.text
		// fill upto mark
		for ; xpos < c.xpos; xpos++ {
			bytes = append(bytes, ' ')
		}
		// Clip chunk if it overlaps
		if c.xpos < xpos {
			diff := xpos - c.xpos
			c_text = c_text.Clip(diff, math.MaxUint)
		}
		// Construct
		bytes = append(bytes, c_text.Bytes()...)
		// Advance cursor
		xpos += c_text.Len()
	}
	// fill to end of line
	for ; xpos < p.width; xpos++ {
		bytes = append(bytes, ' ')
	}
	//
	return bytes
}

type terminalChunk struct {
	xpos uint
	text FormattedText
}

// Construct a line full of blanks.
func blankLine(width uint) []byte {
	bytes := make([]byte, width)
	//
	for i := range bytes {
		bytes[i] = ' '
	}
	//
	return bytes
}

// TerminalInput is responsible for processing input coming from the terminal
// and translating it into key sequences.  Specifically, it must handle escape
// sequences correctly.
type terminalInput struct {
	// Processed input buffer.
	keyBuffer []uint16
}

func newTerminalInput() terminalInput {
	return terminalInput{nil}
}

// ReadKey returns a keyevent from the keyboard.  This is either an ASCII
// character, or an extended escape code.
func (t *terminalInput) Read() (uint16, error) {
	// Sanity check whether key already available
	if len(t.keyBuffer) > 0 {
		key := t.keyBuffer[0]
		t.keyBuffer = t.keyBuffer[1:]
		// Done
		return key, nil
	}
	//
	var buffer []byte
	//
	for {
		var buf [128]byte
		// Read in raw bytes
		n, err := os.Stdin.Read(buf[:])
		// Append bytes
		buffer = append(buffer, buf[:n]...)
		//
		if err != nil {
			return 0, err
		} else if n < 128 {
			break
		}
	}
	// Process raw bytes
	t.processRawBuffer(buffer)
	// Read offshoots of our work
	return t.Read()
}

func (t *terminalInput) processRawBuffer(buffer []byte) {
	for i := 0; i < len(buffer); {
		i += t.processRawBufferByte(buffer[i:])
	}
}

func (t *terminalInput) processRawBufferByte(buffer []byte) int {
	key := uint16(buffer[0])
	// Check for escape
	if key != ESC || len(buffer) == 1 {
		t.keyBuffer = append(t.keyBuffer, key)
		return 1
	} else if buffer[1] == '[' && len(buffer) > 2 {
		return 2 + t.processCsiCommand(buffer[2:])
	} else {
		// Other escapes are currently unsupported.  Therefore, we just append
		// them as raw bytes.
		t.keyBuffer = append(t.keyBuffer, key)
		return 2
	}
}

func (t *terminalInput) processCsiCommand(buffer []byte) int {
	var key uint16
	// Dispatch escape
	switch buffer[0] {
	case 'A':
		key = CURSOR_UP
	case 'B':
		key = CURSOR_DOWN
	case 'C':
		key = CURSOR_RIGHT
	case 'D':
		key = CURSOR_LEFT
	case '5':
		key = SCROLL_UP
	case '6':
		key = SCROLL_DOWN
	case 'Z':
		key = BACKTAB
	default:
		key = UNKNOWN
	}
	// Done
	t.keyBuffer = append(t.keyBuffer, key)
	//
	return 1
}

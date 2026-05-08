// Package lspconv converts between pasta's byte offsets and LSP's
// (line, character) positions.
//
// LSP positions are zero-based. The `character` field counts UTF-16
// code units (per the spec's default `PositionEncodingKind`), not
// bytes or runes. This package provides the conversion.
package lspconv

import (
	"sort"
	"unicode/utf16"
	"unicode/utf8"
)

// Doc is a snapshot of a document's text plus a precomputed line-start
// table. Build one per document edit; reuse it for many byte→position
// conversions in a single diagnostic run.
type Doc struct {
	src        []byte
	lineStarts []int  // byte offset of each line's first character
	asciiOnly  []bool // per line: true if the line is pure ASCII (fast path)
}

// New builds a Doc for src.
func New(src []byte) *Doc {
	starts := []int{0}
	asciiOnly := []bool{}
	lineStart := 0
	asciiThisLine := true
	for i := 0; i < len(src); i++ {
		c := src[i]
		if c >= 0x80 {
			asciiThisLine = false
		}
		if c == '\n' {
			asciiOnly = append(asciiOnly, asciiThisLine)
			starts = append(starts, i+1)
			lineStart = i + 1
			asciiThisLine = true
		}
	}
	// Trailing line (no final newline).
	if lineStart <= len(src) {
		asciiOnly = append(asciiOnly, asciiThisLine)
	}
	return &Doc{src: src, lineStarts: starts, asciiOnly: asciiOnly}
}

// Position is an LSP-style (line, character) pair, zero-based, with
// `character` measured in UTF-16 code units.
type Position struct {
	Line      uint32
	Character uint32
}

// Range is two Positions, half-open [Start, End).
type Range struct {
	Start Position
	End   Position
}

// PositionFromByte converts a byte offset to an LSP Position.
// Out-of-range offsets clamp to the nearest valid position.
func (d *Doc) PositionFromByte(byteOff uint32) Position {
	off := int(byteOff)
	if off < 0 {
		off = 0
	}
	if off > len(d.src) {
		off = len(d.src)
	}
	// Binary search for the line whose start <= off.
	line := sort.Search(len(d.lineStarts), func(i int) bool {
		return d.lineStarts[i] > off
	}) - 1
	if line < 0 {
		line = 0
	}
	lineStart := d.lineStarts[line]
	col := utf16Col(d.src[lineStart:off], d.asciiOnly[line])
	return Position{Line: uint32(line), Character: col}
}

// RangeFromBytes converts a [start, end) byte range to an LSP Range.
func (d *Doc) RangeFromBytes(start, end uint32) Range {
	return Range{
		Start: d.PositionFromByte(start),
		End:   d.PositionFromByte(end),
	}
}

// utf16Col counts UTF-16 code units in s. Fast path for pure ASCII
// lines: byte length == UTF-16 code units.
func utf16Col(s []byte, asciiFastPath bool) uint32 {
	if asciiFastPath {
		return uint32(len(s))
	}
	n := uint32(0)
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRune(s[i:])
		if r == utf8.RuneError && size == 1 {
			n++
			i++
			continue
		}
		if utf16.IsSurrogate(r) || r > 0xFFFF {
			n += 2
		} else {
			n++
		}
		i += size
	}
	return n
}

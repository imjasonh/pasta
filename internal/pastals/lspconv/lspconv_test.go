package lspconv

import "testing"

func TestPositionFromByte(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		byte  uint32
		line  uint32
		char  uint32
	}{
		{"start", "abc\ndef\n", 0, 0, 0},
		{"end of first line", "abc\ndef\n", 3, 0, 3},
		{"start of second line", "abc\ndef\n", 4, 1, 0},
		{"middle of second line", "abc\ndef\n", 6, 1, 2},
		{"after final newline", "abc\ndef\n", 8, 2, 0},
		{"no trailing newline", "abc\ndef", 7, 1, 3},
		{"empty source", "", 0, 0, 0},
		{"out of range clamps", "abc", 99, 0, 3},
		// Multi-byte UTF-8 (é = 0xC3 0xA9, 1 UTF-16 code unit).
		{"after BMP rune", "café\n", 5, 0, 4},
		{"between BMP rune bytes - clamps to next col", "café\n", 4, 0, 4},
		// Supplementary plane: 🎉 = 0xF0 0x9F 0x8E 0x89 (2 UTF-16 code units).
		{"after surrogate-pair rune", "x🎉y", 5, 0, 3},
		{"start of line with surrogate", "x🎉y", 0, 0, 0},
		{"after surrogate followed by ASCII", "x🎉y", 6, 0, 4},
		// Mixed lines.
		{"second line in mixed doc", "ascii\ncafé\n", 6, 1, 0},
		{"after é on second line", "ascii\ncafé\n", 11, 1, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := New([]byte(tc.src))
			p := d.PositionFromByte(tc.byte)
			if p.Line != tc.line || p.Character != tc.char {
				t.Errorf("PositionFromByte(%d) = (%d, %d), want (%d, %d)",
					tc.byte, p.Line, p.Character, tc.line, tc.char)
			}
		})
	}
}


func TestRangeFromBytes(t *testing.T) {
	src := "if x == nil {\n  return\n}"
	// "x" is at byte 3, "nil" ends at byte 11.
	d := New([]byte(src))
	r := d.RangeFromBytes(3, 11)
	if r.Start.Line != 0 || r.Start.Character != 3 {
		t.Errorf("Start = %+v, want line 0 char 3", r.Start)
	}
	if r.End.Line != 0 || r.End.Character != 11 {
		t.Errorf("End = %+v, want line 0 char 11", r.End)
	}
}

func TestAsciiFastPath(t *testing.T) {
	// Verify the ASCII fast path produces the same result as the slow
	// path. Use a long ASCII line.
	src := "package main\n" + "import \"fmt\"\n" + "func main() { fmt.Println(\"hi\") }\n"
	d := New([]byte(src))
	for i := uint32(0); i <= uint32(len(src)); i++ {
		p := d.PositionFromByte(i)
		// For ASCII, character == byte column.
		expectedLine := uint32(0)
		col := uint32(0)
		for j := uint32(0); j < i; j++ {
			if src[j] == '\n' {
				expectedLine++
				col = 0
			} else {
				col++
			}
		}
		if p.Line != expectedLine || p.Character != col {
			t.Errorf("byte %d: got %+v, want line %d char %d", i, p, expectedLine, col)
		}
	}
}

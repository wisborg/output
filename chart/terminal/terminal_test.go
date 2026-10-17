package terminal

import (
	"bytes"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

// A picture is drawn only on a terminal known to show one, outside tmux and
// screen; a mode other than auto is obeyed whatever the output; and only the choice
// that would surprise -- braille in tmux, on a terminal that could show a
// picture -- is explained.
func TestChooseTerminal(t *testing.T) {
	for _, c := range []struct {
		name     string
		mode     Protocol
		env      map[string]string
		tty      bool
		want     Protocol
		withNote bool
	}{
		{"iTerm2", Auto, map[string]string{"TERM_PROGRAM": "iTerm.app"}, true, ITerm, false},
		{"iTerm2 over ssh", Auto, map[string]string{"LC_TERMINAL": "iTerm2"}, true, ITerm, false},
		{"WezTerm", Auto, map[string]string{"TERM_PROGRAM": "WezTerm"}, true, ITerm, false},
		{"kitty", Auto, map[string]string{"TERM": "xterm-kitty"}, true, Kitty, false},
		{"kitty by window", Auto, map[string]string{"KITTY_WINDOW_ID": "1"}, true, Kitty, false},
		{"Ghostty", Auto, map[string]string{"TERM_PROGRAM": "ghostty"}, true, Kitty, false},
		{"Apple's Terminal", Auto, map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, true, Braille, false},
		{"into a pipe", Auto, map[string]string{"TERM_PROGRAM": "iTerm.app"}, false, Braille, false},
		{"iTerm2 in tmux", Auto, map[string]string{"TERM_PROGRAM": "iTerm.app", "TMUX": "/tmp/tmux-501/default,1,0"}, true, Braille, true},
		{"screen", Auto, map[string]string{"LC_TERMINAL": "iTerm2", "TERM": "screen-256color"}, true, Braille, true},
		{"kitty asked for, into a pipe", Kitty, nil, false, Kitty, false},
		{"braille asked for in iTerm2", Braille, map[string]string{"TERM_PROGRAM": "iTerm.app"}, true, Braille, false},
	} {
		how, note, err := Choose(c.mode, func(k string) string { return c.env[k] }, c.tty)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if how != c.want || (note != "") != c.withNote {
			t.Errorf("%s: drawn %s with note %q, want %s (note: %v)", c.name, how, note, c.want, c.withNote)
		}
	}
	if _, _, err := Choose("sixel", func(string) string { return "" }, true); err == nil {
		t.Error("sixel was accepted")
	}
}

// fakePNG is bytes standing in for a PNG, long enough to need several of
// kitty's chunks.
var fakePNG = bytes.Repeat([]byte("\x89PNG not really"), 1000)

// iTerm2's image is the whole file in one escape sequence, the width
// asked for in characters.
func TestITermImage(t *testing.T) {
	var b bytes.Buffer
	if err := WriteITerm(&b, fakePNG, 80); err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile("^\x1b]1337;File=inline=1;size=(\\d+);width=80;preserveAspectRatio=1:([A-Za-z0-9+/=]+)\a\n$").FindStringSubmatch(b.String())
	if m == nil {
		t.Fatalf("not an iTerm2 inline image: %q...", b.String()[:60])
	}
	if got, _ := base64.StdEncoding.DecodeString(m[2]); !bytes.Equal(got, fakePNG) || m[1] != "15000" {
		t.Errorf("the image's bytes or size (%s) are not the file's", m[1])
	}
}

// kitty's image is in chunks no longer than the protocol takes, the first
// saying what it is and asking for no answer, each but the last saying
// more follows; together they are the file.
func TestKittyImage(t *testing.T) {
	var b bytes.Buffer
	if err := WriteKitty(&b, fakePNG, 80); err != nil {
		t.Fatal(err)
	}
	seqs := regexp.MustCompile("\x1b_G([^;]*);([^\x1b]*)\x1b\\\\").FindAllStringSubmatch(b.String(), -1)
	if len(seqs) < 3 {
		t.Fatalf("%d chunks for %d bytes, want several", len(seqs), len(fakePNG))
	}
	var data strings.Builder
	for i, s := range seqs {
		ctrl, chunk := s[1], s[2]
		if len(chunk) > KittyChunk {
			t.Errorf("chunk %d is %d long, over kitty's %d", i, len(chunk), KittyChunk)
		}
		if i == 0 && !strings.HasPrefix(ctrl, "a=T,f=100,q=2,c=80,") {
			t.Errorf("the first chunk says %q; it should transmit a PNG to show at 80 columns, unanswered", ctrl)
		}
		if !strings.Contains(ctrl, "q=2") {
			t.Errorf("chunk %d says %q, which a terminal may answer", i, ctrl)
		}
		if more := strings.HasSuffix(ctrl, "m=1"); more != (i < len(seqs)-1) {
			t.Errorf("chunk %d of %d says m=%v", i, len(seqs), more)
		}
		data.WriteString(chunk)
	}
	if got, _ := base64.StdEncoding.DecodeString(data.String()); !bytes.Equal(got, fakePNG) {
		t.Error("the chunks together are not the file")
	}
}

// Write shows a picture in the protocol asked, and refuses braille.
func TestWrite(t *testing.T) {
	var a, b bytes.Buffer
	if err := Write(&a, ITerm, fakePNG, 80); err != nil || !strings.HasPrefix(a.String(), "\x1b]1337;") {
		t.Errorf("ITerm: %v %q", err, a.String()[:10])
	}
	if err := Write(&b, Kitty, fakePNG, 80); err != nil || !strings.HasPrefix(b.String(), "\x1b_G") {
		t.Errorf("Kitty: %v %q", err, b.String()[:10])
	}
	if err := Write(&bytes.Buffer{}, Braille, fakePNG, 80); err == nil {
		t.Error("braille wrote a picture")
	}
}

// A background is light where COLORFGBG says white or a bright colour, and
// dark where it says a dark one or nothing.
func TestLightBackground(t *testing.T) {
	for v, want := range map[string]bool{"0;15": true, "0;7": true, "0;11": true, "15;0": false, "7;8": false, "": false, "nonsense": false, "15;default;0": false} {
		if got := LightBackground(func(string) string { return v }); got != want {
			t.Errorf("COLORFGBG %q: light %v, want %v", v, got, want)
		}
	}
}

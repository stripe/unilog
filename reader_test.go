package unilog

import (
	"bufio"
	"io"
	"strings"
	"testing"
	"time"
)

type op struct {
	read   int
	expect string
}

func testUnilogReader(t *testing.T) {
	var twoLines = "hello world\nsecond line\n"
	tests := []struct {
		in  string
		ops []op
	}{
		{twoLines, []op{
			{5, "hello"},
			{-1, ""},
			{10, " "},
			{10, "w"},
			{10, "o"},
			{10, "r"},
			{1, "l"},
			{1, "d"},
			{5, "\n"},
			{5, ""},
			{1, ""},
		}},
		{twoLines, []op{
			{12, "hello world\n"},
			{-1, ""},
			{10, ""},
			{1, ""},
		}},
	}
	for i, tc := range tests {
		shutdown := make(chan struct{})
		rd := NewUnilogReader(strings.NewReader(tc.in), shutdown)
		buf := make([]byte, 128)
		for _, o := range tc.ops {
			if o.read < 0 {
				close(shutdown)
				// HACK: Wait for the shutdown to propagate.
				for !rd.(*UnilogReader).shuttingDown {
					time.Sleep(1 * time.Millisecond)
				}
				continue
			}
			if len(buf) < o.read {
				buf = make([]byte, o.read)
			}
			n, e := rd.Read(buf[:o.read])
			if n != len(o.expect) {
				t.Errorf("test %d, read(%d) expected %v, but got %d bytes",
					i, o.read, o.expect, n)
				break
			}
			got := string(buf[:n])
			if got != o.expect {
				t.Errorf("test %d, read(%d) expected %v, but got %v",
					i, o.read, o.expect, got)
				break
			}
			if e != nil && len(o.expect) > 0 {
				t.Errorf("test %d, read(%d) expected %v, but got err: %s",
					i, o.read, o.expect, e.Error())
				break
			}
			if len(o.expect) == 0 && e != io.EOF {
				t.Errorf("expected EOF, got %v", e)
				break
			}
		}
	}
}

type OurReader struct {
	input chan []byte
}

func (or *OurReader) Read(p []byte) (n int, err error) {
	bs, ok := <-or.input
	if !ok {
		return 0, io.EOF
	}
	return copy(p, bs), nil
}

func TestReaderAlone(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		input := make(chan []byte, 1)
		inRdr := OurReader{input}
		shutdown := make(chan struct{}, 1)
		close(shutdown)
		rdr := NewUnilogReader(&inRdr, shutdown)
		bufRdr := bufio.NewReader(rdr)
		input <- []byte("hello I am a line\n")
		line, err := bufRdr.ReadString('\n')
		if err != nil {
			t.Error(err)
		}
		if line != "hello I am a line\n" {
			t.Error(line)
		}
	})

	// Less happy case: shutdown with a missing newline.
	t.Run("complex", func(t *testing.T) {
		input := make(chan []byte)
		inRdr := OurReader{input}
		shutdown := make(chan struct{})
		close(shutdown)
		rdr := NewUnilogReader(&inRdr, shutdown)
		bufRdr := bufio.NewReader(rdr)

		go func() {
			input <- []byte("unterminated")
			input <- []byte("done\n")
		}()
		line, err := bufRdr.ReadString('\n')
		if err != nil {
			t.Error(err)
		}
		if line != "unterminateddone\n" {
			t.Error(line)
		}
	})
}

package unilog

import (
	"io"
	"strings"
	"testing"
	"time"
)

type op struct {
	read   int
	expect string
}

func TestUnilogReader(t *testing.T) {
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

package unilog

import (
	"strings"
	"testing"
)

var shakespeare = []string{
	"To be, or not to be, that is the question-",
	"Whether 'tis Nobler in the mind to suffer",
	"The Slings and Arrows of outrageous Fortune,",
	"Or to take Arms against a Sea of troubles,",
	"And by opposing end them?",
}

func TestReadlines(t *testing.T) {
	r := strings.NewReader(strings.Join(shakespeare, "\n"))
	ch := make(chan struct{})
	defer close(ch)
	lc, _ := readlines(r, 1, ch)
	var i int
	for line := range lc {
		if line != shakespeare[i] {
			t.Errorf("Line %d should have been %q, but got %q", i,
				shakespeare[i], line)
		}
		i++
	}
	if i != len(shakespeare) {
		t.Errorf("Did not get enough lines of shakespeare")
	}
}

var big = strings.Repeat("Unique New York", 9000)

func TestReadlinesWithLongLines(t *testing.T) {
	r := strings.NewReader(big)
	ch := make(chan struct{})
	defer close(ch)

	lc, _ := readlines(r, 1, ch)
	line := <-lc
	if line != big {
		t.Errorf("Lines do not match! Got %d bytes; expected %d",
			len(line), len(big))
	}
	if <-lc != "" {
		t.Error("Did not reach EOF")
	}
}

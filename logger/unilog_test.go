package logger

import (
	"bytes"
	"os"
	"strings"
	"syscall"
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

func TestFilterFunction(t *testing.T) {
	var input = shakespeare[0]
	const expected = "To bee, or not to bee, that ain't the question-\n"

	u := &Unilog{}

	// double the first two instances of the character "e"
	var doubleEFilter = func(s string) string {
		return strings.Replace(s, "e", "ee", 2)
	}

	var isToAintFilter = func(s string) string {
		return strings.Replace(s, "is", "ain't", -1)
	}

	u.Filters = []Filter{
		Filter(doubleEFilter),
		Filter(isToAintFilter),
	}

	result := u.format(input)

	i := strings.Index(result, "]")
	if i == -1 {
		t.Errorf("Expected timestamp but found none")
	}

	// Remove the entire timestamp and the leading space to avoid
	// having to match the timestamp part of the string in the test
	result = result[i+2:]
	if result != expected {
		t.Errorf("expected %q, found %q", expected, result)
	}
}

func TestTwoStateExit(t *testing.T) {
	u := &Unilog{}
	exitCode := -1

	term := make(chan os.Signal, 2)
	quit := make(chan os.Signal, 2)
	exit := make(chan int, 2)

	u.sigTerm = term
	u.sigQuit = quit
	u.exit = func(code int) {
		exitCode = code
		exit <- 1
	}
	u.shutdown = make(chan struct{})

	go u.run()

	term <- syscall.SIGTERM
	<-u.shutdown
	quit <- syscall.SIGQUIT

	<-exit
	if exitCode != 1 {
		t.Error("Did not call exit.")
	}
}

func TestSigTermNoExit(t *testing.T) {
	u := &Unilog{}

	term := make(chan os.Signal, 1)
	quit := make(chan os.Signal, 1)

	u.sigTerm = term
	u.sigQuit = quit
	u.exit = func(code int) {
		t.Error("Called exit.")
	}
	u.shutdown = make(chan struct{})

	term <- syscall.SIGTERM
	if !u.tick() {
		t.Error("Tick returned false.")
	}
}

func TestSigQuitNoOp(t *testing.T) {
	u := &Unilog{}

	term := make(chan os.Signal, 1)
	quit := make(chan os.Signal, 1)

	u.sigTerm = term
	u.sigQuit = quit
	u.exit = func(code int) {
		t.Error("Called exit.")
	}
	u.shutdown = make(chan struct{})

	quit <- syscall.SIGQUIT
	if !u.tick() {
		t.Error("Tick returned false.")
	}
}

type MockClient struct {
	Counts map[string]int64
}

func (mc *MockClient) Count(name string, value int64, tags []string, rate float64) error {
	var buffer bytes.Buffer
	for _, tag := range tags {
		buffer.WriteString("[")
		buffer.WriteString(tag)
		buffer.WriteString("]")
	}
	buffer.WriteString(name)
	mc.Counts[buffer.String()] += value
	return nil
}

func TestNoTags(t *testing.T) {
	client := &MockClient{Counts: make(map[string]int64)}
	for i := 0; i < 100; i++ {
		IndependentCount(client, "metric", 10, nil, 1)
	}
	if client.Counts["metric"] != 1000 {
		t.Errorf("Count was %d, not %d", client.Counts["metric"], 1000)
	}
}

func TestWithGlobalTags(t *testing.T) {
	client := &MockClient{Counts: make(map[string]int64)}
	for i := 0; i < 100; i++ {
		IndependentCount(client, "metric", 10, []string{"foo:bar"}, 1)
		IndependentCount(client, "metric", 5, []string{"baz:qaz", "veneurglobalonly:true"}, 1)
	}
	var tests = map[string]int64{
		"[foo:bar]metric":                        1000,
		"[baz:qaz][veneurglobalonly:true]metric": 500,
	}
	for key, value := range tests {
		if client.Counts[key] != value {
			t.Errorf("Count for %s was %d, not %d", key, client.Counts[key], value)
		}
	}
}

func TestWithIndependentTags(t *testing.T) {
	// Save and set tagState
	tmp := tagState
	tagState = newIndependentTags([]string{"veneurglobalonly:true", "owner:observability"})

	client := &MockClient{Counts: make(map[string]int64)}
	for i := 0; i < 100; i++ {
		IndependentCount(client, "metric", 10, nil, 1)
		IndependentCount(client, "metric", 5, []string{"baz:qaz"}, 1)
	}
	var tests = map[string]int64{
		"metric": 1000,
		"[veneurglobalonly:true]metric.veneurglobalonly":          1000,
		"[owner:observability]metric.owner":                       1000,
		"[baz:qaz]metric":                                         500,
		"[baz:qaz][veneurglobalonly:true]metric.veneurglobalonly": 500,
		"[baz:qaz][owner:observability]metric.owner":              500,
	}
	for key, value := range tests {
		if client.Counts[key] != value {
			t.Errorf("Count for %s was %d, not %d", key, client.Counts[key], value)
		}
	}
	// Restore tagState
	tagState = tmp
}

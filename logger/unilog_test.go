package logger

import (
	"bytes"
	encjson "encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/unilog/filters"
	"github.com/stripe/unilog/json"
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

// double the first two instances of the character "e"
type doubleEFilter struct{}

func (ee *doubleEFilter) FilterLine(line string) string {
	return strings.Replace(line, "e", "ee", 2)
}

func (ee *doubleEFilter) FilterJSON(line *json.LogLine) {
	msg := (*line)["message"].(string)
	(*line)["message"] = strings.Replace(msg, "e", "ee", 2)
}

type isToAintFilter struct{}

func (ee *isToAintFilter) FilterLine(line string) string {
	return strings.Replace(line, "is", "ain't", -1)
}

func (ee *isToAintFilter) FilterJSON(line *json.LogLine) {
	msg := (*line)["message"].(string)
	(*line)["message"] = strings.Replace(msg, "is", "ain't", -1)
}

func TestFilterFunction(t *testing.T) {
	var input = shakespeare[0]
	const expected = "To bee, or not to bee, that ain't the question-\n"

	u := &Unilog{}

	u.Filters = []Filter{
		Filter(&doubleEFilter{}),
		Filter(&isToAintFilter{}),
	}

	result := u.format(input)
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

func TestIndependentTagRace(t *testing.T) {
	tagState = newIndependentTags([]string{"foo:bar"})
	for i := 0; i < 100; i++ {
		go func(num int) {
			tagState.GetTags(fmt.Sprintf("%d", num))
		}(i)
	}
}

func TestSentryPanic(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("Expected panic, but got none")
		} else if !strings.HasPrefix(fmt.Sprintf("%v", r), "Invalid DSN:") {
			t.Errorf("Expected `Invalid DSN: ...`, but got: %s", r)
		}
	}()

	u := &Unilog{SentryDSN: "foo"}
	u.setupSentry()
}

func TestSentrySuccess(t *testing.T) {
	defer func() {
		r := recover()
		if r != nil {
			t.Errorf("Expected no panic, but got: %s", r)
		}
	}()

	u := &Unilog{SentryDSN: "https://123:456@foo/789"}
	u.setupSentry()
}

type mockFile struct {
	buf *bytes.Buffer
}

func (m mockFile) Write(p []byte) (int, error) {
	return m.buf.Write(p)
}
func (mockFile) Close() error {
	return nil
}

func getLogLine(u *Unilog, line string) string {
	var buf bytes.Buffer
	u.file = mockFile{buf: &buf}

	u.logLine(line)
	return buf.String()
}

func getLogJSON(u *Unilog, line string) string {
	var buf bytes.Buffer
	u.file = mockFile{buf: &buf}
	u.jsonEncoder = encjson.NewEncoder(u.file)

	u.logJSON(line)
	return buf.String()
}

func TestLogLine(t *testing.T) {
	out := getLogLine(&Unilog{}, "hi")
	assert.Equal(t, "hi\n", out)

	out = getLogLine(&Unilog{
		Filters: []Filter{
			&filters.TimePrefixFilter{Format: "foo"},
		},
	}, "hi")
	assert.Equal(t, "[foo] hi\n", out)
}

func TestLogJSON(t *testing.T) {
	out := getLogJSON(&Unilog{}, "hi")
	assert.Equal(t, "hi\n", out)

	out = getLogJSON(&Unilog{}, `{"message":"hi"}`)
	assert.Regexp(t, `\{"timestamp":[\d\.]+,"message":"hi"}\n`, out)
}

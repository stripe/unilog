package filters

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/unilog/json"
)

var low time.Time = time.Now()

func TestTimePrefixLine(t *testing.T) {
	f := TimePrefixFilter{}
	str := f.FilterLine("")
	f.Format = time.UnixDate
	str2 := f.FilterLine("")

	between(t, str[1:27], defaultFormat, low, time.Now())
	between(t, str2[1:29], time.UnixDate, low, time.Now())
}

func TestTimePrefixJSON(t *testing.T) {
	f := TimePrefixFilter{}
	m := json.LogLine(map[string]interface{}{})
	f.FilterJSON(&m)

	assert.Equal(t, len(m), 0, "JSON filter should make no map modifications")
}

func between(t *testing.T, check string, format string, bottom, high time.Time) {
	if h, ok := interface{}(t).(interface {
		Helper()
	}); ok {
		h.Helper()
	}

	c, err := time.Parse(format, check)

	if err != nil {
		t.Fatal(fmt.Sprintf("Input string was not a parseable timestamp: %s", check))
	}

	// Converting the timestamps to strings and back - as inherently happens
	// when the filter prints them in the log - seems to introduce a stable
	// inaccuracy specific to the format. So, convert our low and high bounds to
	// and back from strings, then check bounds based on that.
	lstr, hstr := bottom.Format(format), high.Format(format)
	plow, _ := time.Parse(format, lstr)
	phigh, _ := time.Parse(format, hstr)

	assert.True(t, (c.After(plow) || c.Equal(plow)), "Input (%q) was below lower bound (%q) by %dns", check, plow.Format(format), plow.Sub(c))
	assert.True(t, (phigh.After(c) || c.Equal(phigh)), "Input (%q) was above upper bound (%q) by %dns", check, phigh.Format(format), c.Sub(phigh))
}

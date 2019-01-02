package json

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTS(t *testing.T) {
	tests := []struct {
		inputTS  string
		now      bool
		outputTS string
	}{
		{"2006-01-02T15:04:05.999999999Z", false, "2006-01-02T15:04:05.999999999Z"},
		{"2006-01-02T15:04:05Z", false, "2006-01-02T15:04:05Z"},
		{"Mon, 02 Jan 2006 15:04:05 -0700", false, "2006-01-02T15:04:05Z"},
		{"gibberish", true, ""},
	}
	nowish := time.Now()
	layout := "2006-01-02T15:04:05.999999999Z"
	for _, elt := range tests {
		test := elt
		name := fmt.Sprintf("%s", elt.inputTS)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			line := LogLine{"ts": test.inputTS}
			ts := line.TS()
			if !test.now {
				assert.False(t, nowish.Before(ts),
					"timestamp %v should be an actual timestamp, not time.Now()",
					nowish,
				)
				out := ts.Format(layout)
				assert.Equal(t, test.outputTS, out)
			} else {
				assert.True(t, nowish.Before(ts),
					"timestamp %v should be sometime after the start of the test %v",
					nowish, ts)
			}
		})
	}
}

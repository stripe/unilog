package json

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestMarshal(t *testing.T) {
	tests := []struct {
		in string
	}{
		{`{"msg":"hi", "ts":"2006-01-02T15:04:05.999999999Z"}`},
		{`{"msg":"hi"}`},
		{`{"what":"no",    "teletubbies":["boo", "lala"]}`},
	}
	for _, elt := range tests {
		test := elt
		t.Run(test.in, func(t *testing.T) {
			t.Parallel()
			var line LogLine
			err := json.Unmarshal(([]byte)(test.in), &line)
			require.NoError(t, err)

			out, err := json.Marshal(line)
			outstr := (string)(out)
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(outstr, `{"ts":"`), outstr)

			var roundtrip LogLine
			err = json.Unmarshal(out, &roundtrip)
			assert.NoError(t, err)
		})
	}
}

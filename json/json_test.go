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
		inputTS interface{}
		now     bool
		epochS  int64
		epochNS int64
	}{
		{"2006-01-02T15:04:05.999999999Z", false, 1136214245, 999999999},
		{"2006-01-02T15:04:05Z", false, 1136214245, 0},
		{"Mon, 02 Jan 2006 15:04:05 -0700", false, 1136239445, 0},
		{"gibberish", true, 0, 0},
		{1550493962.283873, false, 1550493962, 283873010},
	}
	nowish := time.Now()
	for _, elt := range tests {
		test := elt
		name := fmt.Sprintf("%v", elt.inputTS)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			line := LogLine{"timestamp": test.inputTS}
			ts := line.Timestamp()
			if !test.now {
				assert.False(t, nowish.Before(ts),
					"timestamp %v should be an actual timestamp, not time.Now()",
					nowish,
				)
				epoch := time.Unix(test.epochS, test.epochNS)
				assert.WithinDuration(t, epoch, ts, time.Microsecond)

				// Try round-tripping the TS through Marshal:
				b, err := json.Marshal(&line)
				require.NoError(t, err)

				var roundtrip LogLine
				err = json.Unmarshal(b, &roundtrip)
				require.NoError(t, err)

				t.Logf("log line: %s", string(b))
				assert.WithinDuration(t, epoch, roundtrip.Timestamp(), time.Microsecond)
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
		{`{"msg":"hi", "timestamp":"2006-01-02T15:04:05.999999999Z"}`},
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
			assert.True(t, strings.HasPrefix(outstr, `{"timestamp":`), outstr)

			var roundtrip LogLine
			err = json.Unmarshal(out, &roundtrip)
			assert.NoError(t, err)
		})
	}
}

type unwritable struct{}

func (j unwritable) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("I'm a bobby tables \" error")
}

func TestMarshalBrokenFields(t *testing.T) {
	line := LogLine{
		"this_is_broken": unwritable{},
		"this_works":     "hi there",
	}

	out, err := json.Marshal(line)
	outstr := (string)(out)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(outstr, `{"timestamp":`), outstr)

	t.Log(outstr)

	var roundtrip LogLine
	err = json.Unmarshal(out, &roundtrip)
	assert.NoError(t, err)
	assert.Contains(t, roundtrip["this_is_broken"], "[unilog json marshal error:")
	assert.Equal(t, "hi there", roundtrip["this_works"])
}

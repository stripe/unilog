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
		inputTS string
		now     bool
		epochS  int64
		epochNS int64
	}{
		{`"2006-01-02T15:04:05.999999999Z"`, false, 1136214245, 999999999},
		{`"2006-01-02T15:04:05Z"`, false, 1136214245, 0},
		{`"Mon, 02 Jan 2006 15:04:05 -0700"`, false, 1136239445, 0},
		{`"gibberish"`, true, 0, 0},
		{`1550493962.283873`, false, 1550493962, 283873010},
		{`1550493962`, false, 1550493962, 0},
	}
	nowish := time.Now()
	for _, elt := range tests {
		test := elt
		name := fmt.Sprintf("%v", elt.inputTS)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			jsonText := fmt.Sprintf(`{"timestamp":%s,"foo":"bar"}`, test.inputTS)
			var line LogLine
			err := json.Unmarshal([]byte(jsonText), &line)
			require.NoError(t, err)

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

const lineWithoutTS = `{"trace_id":"6527664022527835840","id":"6045760440938957111","parent_id":"8576604763011053087","start_timestamp":1554825181.8588214,"end_timestamp":1554825181.8588665,"duration_ns":44890,"error":false,"service":"veneur","tags":{"alerting_host_group":"localdnscritical","availability-zone":"us-west-2c","host_cluster":"northwest","host_contact":"developer-platform","host_domain":"stripe.io","host_env":"prod","host_lsbdistcodename":"xenial","host_set":"full-b","host_type":"apibox","instance-type":"m5d.12xlarge"},"indicator":false,"name":"http.gotFirstByte"}`
const lineWithTimestamp = `{"trace_id":"6527664022527835840","id":"6045760440938957111","parent_id":"8576604763011053087","start_timestamp":1554825181.8588214,"end_timestamp":1554825181.8588665,"duration_ns":44890,"error":false,"service":"veneur","tags":{"alerting_host_group":"localdnscritical","availability-zone":"us-west-2c","host_cluster":"northwest","host_contact":"developer-platform","host_domain":"stripe.io","host_env":"prod","host_lsbdistcodename":"xenial","host_set":"full-b","host_type":"apibox","instance-type":"m5d.12xlarge"},"indicator":false,"name":"http.gotFirstByte","timestamp":"2006-01-02T15:04:05.999999999Z"}`
const lineWithTS = `{"trace_id":"6527664022527835840","id":"6045760440938957111","parent_id":"8576604763011053087","start_timestamp":1554825181.8588214,"end_timestamp":1554825181.8588665,"duration_ns":44890,"error":false,"service":"veneur","tags":{"alerting_host_group":"localdnscritical","availability-zone":"us-west-2c","host_cluster":"northwest","host_contact":"developer-platform","host_domain":"stripe.io","host_env":"prod","host_lsbdistcodename":"xenial","host_set":"full-b","host_type":"apibox","instance-type":"m5d.12xlarge"},"indicator":false,"name":"http.gotFirstByte","ts":"2006-01-02T15:04:05.999999999Z"}`

// BenchmarkLineRoundtrip roundtrips each log event through the main
// json decoding & re-encoding machinery: parsing the JSON line,
// extracting or generating a timestamp, serializing it to bytes
// again.
func BenchmarkLineRoundtrip(b *testing.B) {
	tests := []struct{ name, line string }{
		{"without_ts", lineWithoutTS},
		{"with_timestamp", lineWithTimestamp},
		{"with_ts", lineWithTS},
	}
	for _, elt := range tests {
		test := elt
		b.Run(test.name, func(b *testing.B) {
			lineBytes := []byte(test.line)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var ll LogLine
				err := json.Unmarshal(lineBytes, &ll)
				require.NoError(b, err)

				_, err = json.Marshal(ll)
				require.NoError(b, err)
			}

		})
	}
}

// BenchmarkLineRoundtripPlain roundtrips each log event through
// json.Unmarshal/Marshal for a plain map[string]interface{},
// simulating a unilog that doesn't parse/extract timestamps and
// doesn't re-arrange the fields to be in a particular order via a
// custom marshaller.  It is meant to provide a reference point for
// gauging how much processing power the real json-processing
// machinery (and timestamp synthesis) take.
func BenchmarkLineRoundtripPlain(b *testing.B) {
	tests := []struct{ name, line string }{
		{"without_ts", lineWithoutTS},
		{"with_timestamp", lineWithTimestamp},
		{"with_ts", lineWithTS},
	}
	for _, elt := range tests {
		test := elt
		b.Run(test.name, func(b *testing.B) {
			lineBytes := []byte(test.line)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var ll map[string]interface{}
				err := json.Unmarshal(lineBytes, &ll)
				require.NoError(b, err)

				_, err = json.Marshal(ll)
				require.NoError(b, err)
			}

		})
	}
}

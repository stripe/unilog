// Package json provides a type and helpers for representing JSON log
// lines.
//
// Format
//
// The JSON log line format consists of JSON objects, one per
// \n-terminated line.
//
// Unilog recognizes three fields in that object that are considered
// special (all are optional):
//
//    - ts: The time stamp of an event
//    - canonical: Identifies the log event as "canonical", i.e. the
//      most important line a service can log. It is considered to have
//      the highest criticality level.
//    - clevel: The criticality level of the event.
//
// Example
//
//    {"ts":"2006-01-02T15:04:05.999Z07:00","message":"hi there"}
package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// JSONLogLine is a representation of a generic log line that unilog
// can destructure.
type LogLine map[string]interface{}

const defaultTimeFormat = time.RFC3339Nano

// TS returns the timestamp of a log line; if a timestamp is set, TS
// will attempt to parse it (first according to time.RFC3339Nano and
// then time.RFC1123Z); if no timestamp is present, or the present
// time stamp can not be parsed, TS returns the current time.
func (j *LogLine) TS() time.Time {
	if tsS, ok := (*j)["ts"]; ok {
		tsS, ok := tsS.(string)
		if !ok {
			return time.Now()
		}
		// We have a ts, let's try and parse it:
		ts, err := time.Parse(time.RFC3339Nano, tsS)
		if err == nil {
			return ts
		}
		ts, err = time.Parse(time.RFC1123Z, tsS)
		if err == nil {
			return ts
		}
	}
	return time.Now()
}

// MarshalJSON writes the log line in a specific format that's
// optimized for splunk ingestion: First, it writes the timestamp,
// followed by all the other fields.
func (j LogLine) MarshalJSON() ([]byte, error) {
	b := bytes.NewBuffer(([]byte)("{"))
	b.Grow(len(j) * 15) // very naive assumption: average key/value pair is 15 bytes long.
	b.WriteString(`"ts":"`)
	ts := j.TS().Format(defaultTimeFormat)
	b.WriteString(ts)
	b.WriteString(`"`)
	for k, v := range j {
		if k == "ts" {
			continue
		}
		b.WriteString(",")
		kJSON, _ := json.Marshal(k)
		vJSON, err := json.Marshal(v)
		if err != nil {
			vJSON, _ = json.Marshal(fmt.Sprintf(`[unilog json marshal error: %v]`, err))
		}
		b.Write(kJSON)
		b.WriteString(":")
		b.Write(vJSON)
	}
	b.WriteString("}")
	return b.Bytes(), nil
}

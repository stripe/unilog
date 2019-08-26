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
//    - timestamp: The time stamp of an event. Unilog understands both
//      epoch timestamps as float, or RFC3339Nano-formatted
//      timestamps. The timestamp will be normalized to
//      nanosecond-resolution float "timestamp" fields.
//    - canonical: Identifies the log event as "canonical", i.e. the
//      most important line a service can log. It is considered to have
//      the highest criticality level.
//    - clevel: The criticality level of the event.
//
// Example
//
//    {"timestamp":"2006-01-02T15:04:05.999Z07:00","message":"hi there"}
//    {"timestamp":"2006-01-02T15:04:05.999Z07:00","message":"hi there"}
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

const timestampField = "timestamp"

var tsFields = []string{
	timestampField,
	"ts",
}

// Timestamp returns the timestamp of a log line; if a timestamp is
// set on the line, Timestamp will attempt to interpret it
// (integers/floats as UNIX epochs with fractional sub-second
// components, and strings first according to time.RFC3339Nano and
// then time.RFC1123Z). If no timestamp is present, or the present
// time stamp can not be parsed, Timestamp returns the current time.
func (j *LogLine) Timestamp() time.Time {
	for _, tsField := range tsFields {
		if tsS, ok := (*j)[tsField]; ok {
			// We support two different kinds of
			// timestamps here: UNIX epoch timestamps as
			// floats, and RFC3339Nano for strings:
			switch tsV := tsS.(type) {
			case string:
				ts, err := time.Parse(time.RFC3339Nano, tsV)
				if err == nil {
					return ts
				}
				ts, err = time.Parse(time.RFC1123Z, tsV)
				if err == nil {
					return ts
				}
			case float64:
				epochInt := int64(tsV)
				nsec := int64((tsV - float64(epochInt)) * 1000000000)
				return time.Unix(epochInt, nsec)
			default:
				return time.Now()
			}
		}
	}
	return time.Now()
}

// Holds the starting `{`, timestamp field name and field separator
// prefix for the timestamp value.
var encodePrefix []byte

func init() {
	encodePrefix = []byte(fmt.Sprintf(`{"%s":`, timestampField))
}

// MarshalJSON writes the log line in a specific format that's
// optimized for splunk ingestion: First, it writes the timestamp as a
// float UNIX epoch, followed by all the other fields.
func (j LogLine) MarshalJSON() ([]byte, error) {
	b := bytes.NewBuffer(encodePrefix)
	b.Grow(len(j) * 15) // very naive assumption: average key/value pair is 15 bytes long.

	nsepoch := j.Timestamp().UnixNano()
	sec := time.Duration(nsepoch) / time.Second
	usec := (time.Duration(nsepoch) - (sec * time.Second)) / time.Nanosecond
	fmt.Fprintf(b, "%d.%09d", sec, usec)

	for k, v := range j {
		if k == timestampField {
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

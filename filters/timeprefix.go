package filters

import (
	"fmt"
	"time"

	"github.com/stripe/unilog/json"
)

const defaultFormat = "2006-01-02 15:04:05.000000"

// TimePrefixFilter prepends a timestamp onto each event line using the specified
// format string, plus an optional newline.
type TimePrefixFilter struct {
	Format string
}

// FilterLine prepends the current time, in square brackets with a separating
// space, to the provided log line.
func (f TimePrefixFilter) FilterLine(line string) string {
	return fmt.Sprintf("[%s] %s", time.Now().Format(f.getTimeFormat()), line)
}

// FilterJSON is a no-op - TimePrefixFilter does nothing on JSON logs (for now!).
func (f TimePrefixFilter) FilterJSON(line *json.LogLine) {}

func (f TimePrefixFilter) getTimeFormat() string {
	if f.Format != "" {
		return f.Format
	}

	return defaultFormat
}

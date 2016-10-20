package unilog

import (
	"fmt"
	"regexp"
)

var hostnameRegex = regexp.MustCompile(`^([A-Za-z]+)`)

// ParseHostname parses a hostname and returns the
// host type (e.g. mybox1 and mybox-123.local will both
// return 'mybox').
func ParseHostname(hostname string) (string, error) {
	matches := hostnameRegex.FindStringSubmatch(hostname)
	if len(matches) < 2 {
		return "", fmt.Errorf("could not parse %s", hostname)
	}
	return matches[1], nil
}

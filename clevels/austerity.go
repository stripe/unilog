package clevels

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-go/statsd"
)

var Stats *statsd.Client

//go:generate stringer -type=AusterityLevel
type AusterityLevel int

// There are four levels of logs. The names and definitions are adapted
// from Google's usage.
//
// For more information, see Chapter 21 of Site Reliability Engineering
// by Beyer, Jones, Petoff, and Murphy
//
// Sheddable is the least-important, and we can expect frequent partial unavailability
// of Sheddable logs as well as occassional full unavailability without
// disruption.
//
// SheddablePlus is the default level, and we can expect occasional partial availability
// and rare full unavailability.
//
// Critical is used for lines for which partial unavailability is rare.
//
// CriticalPlus is used for lines which should only be unavailable
// in the most extreme circumstances.
const (
	Sheddable AusterityLevel = iota
	SheddablePlus
	Critical
	CriticalPlus
)

// Define two helper constants here. Since our default policies may change
// later, this means we only have to change the definitions at one point

// The default criticality line for any log line
// that doesn't have an explicit clevel tag
const DefaultCriticality AusterityLevel = SheddablePlus

// The default system austerity level, in case
// we are unable to determine it from the state file
const DefaultAusterity AusterityLevel = Sheddable

func (a AusterityLevel) AusterityLevel() string {
	return ""
}

// AusterityBuffer ensures that high-volume services will never block
// when trying to read the system austerity level, even if the process
// that updates the cache is slow. The tradeoff is that, when the system
// austerity level is changed, it will read an extra $AusterityBuffer log
// lines before the change takes effect.
const AusterityBuffer = 100

var CacheInterval = 30 * time.Second

// To determine the current austerity level, read from SystemAusterityLevel.
// Austerity is updated according to CacheInterval,
// and this channel is buffered (size AusterityBuffer) to avoid
// blocking a critical code path.
// As a result, services which have a low log volume will take longer
// to have their austerity level changes come into effect.
var SystemAusterityLevel = make(chan AusterityLevel, AusterityBuffer)

// AusterityFile is the full path to a file that contains the current
// system austerity level.
var AusterityFile string

var InvalidAusterityLevel = errors.New("Invalid austerity level")

// LoadLevel loads the AusterityFile and parses it to determine
// the system austerity level. If it encounters an error, it will
// return the DefaultAusterity.
func LoadLevel() (AusterityLevel, error) {
	f, err := os.Open(AusterityFile)
	if err != nil {
		return DefaultAusterity, err
	}
	defer f.Close()
	level, err := ParseLevel(f)

	return level, err
}

var canonicalRegex = regexp.MustCompile(`CANONICAL-\w+?-LINE`)
var cLevelRegex = regexp.MustCompile(`\[clevel: (\w*?)\]`)
var cLevelChalkRegex = regexp.MustCompile(`\sclevel=(\w+?)\b`)

var clevelRegexes = []*regexp.Regexp{cLevelRegex, cLevelChalkRegex}

// criticality parses the criticality level
// of a log line. Defaults to the value of DefaultCriticality.
func Criticality(line string) AusterityLevel {
	// Things like CANONICAL-API-LINE should never be dropped
	if canonicalRegex.MatchString(line) {
		return CriticalPlus
	}

	for _, r := range clevelRegexes {
		matches := r.FindStringSubmatch(line)
		if len(matches) < 2 {
			continue
		}

		level, err := ParseLevel(strings.NewReader(matches[1]))
		// we don't really care about any error here
		// and want to default to not dropping anything
		if err != nil {
			continue
		}
		// if we've made it this far, we've succesfully parsed
		// the clevel
		return level
	}

	return DefaultCriticality
}

func ParseLevel(r io.Reader) (AusterityLevel, error) {
	bts, err := ioutil.ReadAll(r)
	if err != nil {
		return DefaultAusterity, err
	}

	level := strings.ToLower(strings.TrimSpace(string(bts)))
	switch level {
	case strings.ToLower(Sheddable.String()):
		return Sheddable, nil
	case strings.ToLower(SheddablePlus.String()):
		return SheddablePlus, nil
	case strings.ToLower(Critical.String()):
		return Critical, nil
	case strings.ToLower(CriticalPlus.String()):
		return CriticalPlus, nil
	}
	return DefaultAusterity, InvalidAusterityLevel
}

func SendSystemAusterityLevel() {
	// This is the cached austerity level
	// that will be sent anytime Unilog requests the austerity level,
	// so that there is never any delay.
	// By default, there is no austerity.
	var currentLevel = Sheddable

	// shadow this variable so we can override it in tests, if needed.
	// multiple goroutines write to SystemAusterityLevel in tests, so this shadowing
	// gives us the ability to separate the channels in tests.
	var _SystemAusterityLevel = SystemAusterityLevel

	newLevelCh := make(chan AusterityLevel)
	go func(updatedLevel chan<- AusterityLevel) {
		c := time.Tick(CacheInterval)
		for _ = range c {
			l, err := LoadLevel()
			if err != nil {
				if Stats != nil {
					Stats.Count("unilog.errors.load_level", 1, nil, 1)
				}
				continue
			}
			updatedLevel <- l
		}
	}(newLevelCh)

	// Loop forever on this
	for {
		// This select statement will be on a hotpath
		// so it should never block for long
		select {
		// Continuously send the current austerity level
		// to whoever asks for it
		case _SystemAusterityLevel <- currentLevel:

		case newLevel := <-newLevelCh:
			go ReportAusterity(newLevel)
			currentLevel = newLevel
		}
	}
}

func ReportAusterity(l AusterityLevel) {
	if Stats != nil {
		Stats.Gauge("unilog.austerity.box", float64(l), nil, 1)
	}
}

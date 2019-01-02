package filters

import (
	"math"
	"math/rand"
	"sync"

	"github.com/stripe/unilog/clevels"
	"github.com/stripe/unilog/logger"
)

var startSystemAusterityLevel sync.Once

// AusterityFilter applies system-wide austerity levels to a log line. If austerity levels indicate a line should be shedded, the
type AusterityFilter struct{}

func (a AusterityFilter) setup() {
	// Start austerity level loop sender in goroutine just once
	startSystemAusterityLevel.Do(func() {
		go clevels.SendSystemAusterityLevel()
	})
}

func (a AusterityFilter) FilterLine(line string) string {
	a.setup()
	if shouldShed(clevels.Criticality(line)) {
		return "(shedded)"
	}
	return line
}

func (a AusterityFilter) FilterJSON(line *logger.JSONLogLine) {
	a.setup()
	if shouldShed(clevels.JSONCriticality(line)) {
		// clear the line:
		newLine := map[string]interface{}{}
		if ts, ok := line["ts"]; ok {
			newLine["ts"] = ts
		}
		newLine["shedded"] = true
		*line = newLine
	}
}

func shouldShed(criticalityLevel clevels.AusterityLevel) bool {
	austerityLevel := <-clevels.SystemAusterityLevel
	if criticalityLevel >= austerityLevel {
		return false
	}

	return rand.Float64() > samplingRate(austerityLevel, criticalityLevel)
}

// samplingRate calculates the rate at which loglines will be sampled for the
// given criticality level and austerity level. For example, if the austerity level
// is Critical (3), then lines that are Sheddable (0) will be sampled at .001.
func samplingRate(austerityLevel, criticalityLevel clevels.AusterityLevel) float64 {
	if criticalityLevel > austerityLevel {
		return 1
	}

	levelDiff := austerityLevel - criticalityLevel
	samplingRate := math.Pow(10, float64(-levelDiff))

	return samplingRate
}

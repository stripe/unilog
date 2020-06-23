package filters

import (
	"math"
	"math/rand"
	"sync"

	"github.com/stripe/unilog/clevels"
	"github.com/stripe/unilog/json"
)

var startSystemAusterityLevel sync.Once

// AusterityFilter applies system-wide austerity levels to a log
// line. If austerity levels indicate a line should be shedded, the
// like's contents will replaced with "(shedded)" in text mode and the
// "shedded"=true attribude in JSON mode.
//
// Shedding log lines retains their time stamps.
type AusterityFilter struct{}

// AusteritySetup starts the parser for the system austerity level. It is
// exported so that tests can call it with testing=true in test setup,
// which will disable sending the austerity level (and also appease
// the race detector).
func AusteritySetup(testing bool) {
	// Start austerity level loop sender in goroutine just once
	startSystemAusterityLevel.Do(func() {
		if !testing {
			go clevels.SendSystemAusterityLevel()
		}
	})
}

// FilterLine applies shedding to a text event
func (a AusterityFilter) FilterLine(line string) string {
	AusteritySetup(false)
	if ShouldShed(clevels.Criticality(line)) {
		return "(shedded)"
	}
	return line
}

// FilterJSON applies shedding to a JSON event
func (a AusterityFilter) FilterJSON(line *json.LogLine) {
	AusteritySetup(false)
	if ShouldShed(clevels.JSONCriticality(*line)) {
		// clear the line:
		newLine := map[string]interface{}{}
		if ts, ok := (*line)["ts"]; ok {
			newLine["ts"] = ts
		}
		newLine["shedded"] = true
		*line = newLine
	}
}

// ShouldShed returns true if the given criticalityLevel indicates a log
// should be shed, according to the system austerity level
func ShouldShed(criticalityLevel clevels.AusterityLevel) bool {
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

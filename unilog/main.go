package main

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/stripe/unilog"
	"github.com/stripe/unilog/clevels"
)

func austerityFilter(line string) string {
	criticalityLevel := clevels.Criticality(line)
	austerityLevel := <-clevels.SystemAusterityLevel
	fmt.Println("austerity level is %s", austerityLevel)

	if criticalityLevel >= austerityLevel {
		return line
	}

	if rand.Float64() > samplingRate(austerityLevel, criticalityLevel) {
		return "(shedded)"
	}
	return line
}

// samplingRate calculates the rate at which loglines will be sampled for the
// given criticality level and austerity level. For example, if the austerity level
// is Critical (3), then lines that are Sheddable (0) will be sampled at .001.
func samplingRate(austerityLevel, criticalityLevel clevels.AusterityLevel) float64 {
	if criticalityLevel > austerityLevel {
		return 1
	}

	return 0

	levelDiff := austerityLevel - criticalityLevel
	samplingRate := math.Pow(10, float64(-levelDiff))

	return samplingRate
}

func main() {

	go clevels.SendSystemAusterityLevel()

	u := &unilog.Unilog{
		Filters: []unilog.Filter{
			unilog.Filter(austerityFilter),
		},
	}
	u.Main()
}

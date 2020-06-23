package filters

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/unilog/clevels"
	"github.com/stripe/unilog/json"
)

func TestCalculateSamplingRate(t *testing.T) {
	type CalculateSamplingLevel struct {
		Name        string
		Austerity   clevels.AusterityLevel
		Criticality clevels.AusterityLevel
		Expected    float64
	}

	cases := []CalculateSamplingLevel{
		{
			Name:        "log level higher than austerity",
			Austerity:   clevels.Sheddable,
			Criticality: clevels.SheddablePlus,
			Expected:    1.0,
		},
		{
			Name:        "log level one lower than austerity",
			Austerity:   clevels.Critical,
			Criticality: clevels.SheddablePlus,
			Expected:    0.1,
		},
		{
			Name:        "log level two lower than austerity",
			Austerity:   clevels.Critical,
			Criticality: clevels.Sheddable,
			Expected:    0.01,
		},
		{
			Name:        "log level three lower than austerity",
			Austerity:   clevels.CriticalPlus,
			Criticality: clevels.Sheddable,
			Expected:    0.001,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			samplingRate := samplingRate(tc.Austerity, tc.Criticality)
			assert.Equal(t, tc.Expected, samplingRate)
		})
	}
}

func TestAusterityFilter(t *testing.T) {
	// Make sure SendSystemAusterityLevel is called before we override
	// the underlying channel below
	a := AusterityFilter{}
	AusteritySetup(true)

	clevels.SystemAusterityLevel = make(chan clevels.AusterityLevel)

	line := fmt.Sprintf("some random log line! clevel=%s", clevels.SheddablePlus)

	kill := make(chan struct{})

	go func() {
		for {
			select {
			case clevels.SystemAusterityLevel <- clevels.Critical:
			case <-kill:
				return
			}
		}
	}()

	// seed rand deterministically
	rand.Seed(17)

	// count number of lines dropped
	dropped := 0
	var outputtedLine string

	// now sample out the line a bunch!
	for i := 0; i < 10000; i++ {
		outputtedLine = a.FilterLine(line)
		if strings.Contains(outputtedLine, "(shedded)") {
			dropped++
		}
	}

	// this number is deterministic because rand is seeded & deterministic
	// TODO (kiran, 2016-12-06): maybe add an epsilon
	assert.Equal(t, 8983, dropped)
	kill <- struct{}{}
}

func TestAusterityJSON(t *testing.T) {
	// Make sure SendSystemAusterityLevel is called before we override
	// the underlying channel below
	a := AusterityFilter{}
	AusteritySetup(true)
	clevels.SystemAusterityLevel = make(chan clevels.AusterityLevel)
	kill := make(chan struct{})

	go func() {
		for {
			select {
			case clevels.SystemAusterityLevel <- clevels.Critical:
			case <-kill:
				return
			}
		}
	}()

	// seed rand deterministically
	rand.Seed(17)

	// count number of lines dropped
	dropped := 0

	// now sample out the line a bunch!
	for i := 0; i < 10000; i++ {
		line := json.LogLine{"message": "some random log line!", "clevel": "sheddableplus"}
		a.FilterJSON(&line)
		if _, ok := line["message"]; !ok {
			dropped++
		}
	}

	// this number is deterministic because rand is seeded & deterministic
	assert.Equal(t, 8983, dropped)
	kill <- struct{}{}
}

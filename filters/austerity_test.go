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

func TestAusterityFilter(t *testing.T) {
	// Make sure SendSystemAusterityLevel is called before we override
	// the underlying channel below
	a := AusterityFilter{}
	a.Setup(true)

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
	a.Setup(true)
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

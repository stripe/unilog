package clevels

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseAusterityLevel(t *testing.T) {
	type ParseAusterityTestCase struct {
		Contents      string
		Expected      AusterityLevel
		ExpectedError error
	}

	cases := []ParseAusterityTestCase{
		{
			Contents: "Sheddable",
			Expected: Sheddable,
		},
		{
			Contents: "SheddablePlus",
			Expected: SheddablePlus,
		},
		{
			Contents: "Critical",
			Expected: Critical,
		},
		{
			Contents: "CriticalPlus",
			Expected: CriticalPlus,
		},
		{
			Contents:      "InvalidAusterity",
			Expected:      Sheddable,
			ExpectedError: InvalidAusterityLevel,
		},
		{
			// test case-insensitivity
			Contents: "sHedDaBlePlUs",
			Expected: SheddablePlus,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Contents, func(t *testing.T) {
			l, err := ParseLevel(strings.NewReader(tc.Contents))
			if tc.ExpectedError == nil {
				assert.NoError(t, err)
			} else {
				assert.Equal(t, err, InvalidAusterityLevel)
			}
			assert.Equal(t, tc.Expected, l)
		})
	}
}

// Test that SendAusterityLevel still works properly
// if loading the file returns an error
func TestLoadLevelError(t *testing.T) {
	// For tests, we don't actually want to implement a delay
	// Since both channels will always be ready to send,
	// the scheduler will choose pseudorandomly between the two
	CacheInterval = 0 * time.Millisecond

	// Assert that the file doesn't actually exist, so it should return an error
	// of type *os.PathError
	l, err := LoadLevel()

	// Check the default austerity level
	// For now, if we encountered an error, we should default
	// to the lowest possible level.
	// This may change at some point later.
	assert.Equal(t, Sheddable, l)
	assert.Error(t, err)
	assert.IsType(t, &os.PathError{}, err)

	go SendSystemAusterityLevel()

	for i := 0; i < 10; i++ {
		lvl := <-SystemAusterityLevel

		assert.Equal(t, Sheddable, lvl)
	}
}

func TestCriticality(t *testing.T) {
	type CriticalityTestCase struct {
		name  string
		line  string
		level AusterityLevel
	}
	cases := []CriticalityTestCase{
		{
			name: "CANONICAL-API-LINE",
			// actual CANONICAL-API-LINE, with explicit merchant-identifying tokens removed
			line:  `[2016-11-10 19:18:05.844100] [98381|f1.northwest-1.apiori.com/EzBDuA4iNq-2631925524 85137cc252d87354>e9b8c49860f01f15] CANONICAL-API-LINE: api_method=AccountRetrieveMethod content_type="application/x-www-form-urlencoded" created=1478805073.5253563 http_method=GET ip="54.xxx.xxx.xxx" path="/v1/accounts/acct_xxxxxxxxxxxxxxxx" user_agent="Stripe/v1 RubyBindings/1.31.0" request_id=req_xxxxxxxxxxxxxx response_stripe_version="2016-03-07" status=200 merchant=acct_xxxxxxxxxxxxx`,
			level: CriticalPlus,
		},
		{
			name: "CANONICAL-OTHER-CRITICAL-LINE",
			// actual CANONICAL-API-LINE, with explicit merchant-identifying tokens removed
			line:  `[2016-11-10 19:18:05.844100] [98381|f1.northwest-1.apiori.com/EzBDuA4iNq-2631925524 85137cc252d87354>e9b8c49860f01f15] CANONICAL-OTHER-CRITICAL-LINE: api_method=AccountRetrieveMethod content_type="application/x-www-form-urlencoded" created=1478805073.5253563 http_method=GET ip="54.xxx.xxx.xxx" path="/v1/accounts/acct_xxxxxxxxxxxxxxxx" user_agent="Stripe/v1 RubyBindings/1.31.0" request_id=req_xxxxxxxxxxxxxx response_stripe_version="2016-03-07" status=200 merchant=acct_xxxxxxxxxxxxx`,
			level: CriticalPlus,
		},
		{
			name: "CANONICAL-API-LINE",
			// actual CANONICAL-API-LINE, with explicit merchant-identifying tokens removed
			// this should be criticalplus, despite clevel=sheddable being set
			line:  `[2016-11-10 19:18:05.844100] [98381|f1.northwest-1.apiori.com/EzBDuA4iNq-2631925524 85137cc252d87354>e9b8c49860f01f15] CANONICAL-API-LINE: api_method=AccountRetrieveMethod content_type="application/x-www-form-urlencoded" created=1478805073.5253563 http_method=GET ip="54.xxx.xxx.xxx" path="/v1/accounts/acct_xxxxxxxxxxxxxxxx" user_agent="Stripe/v1 RubyBindings/1.31.0" request_id=req_xxxxxxxxxxxxxx response_stripe_version="2016-03-07" status=200 merchant=acct_xxxxxxxxxxxxx clevel=sheddable`,
			level: CriticalPlus,
		},
		{
			name: "CANONICAL-ADMIN-LINE",
			// actual CANONICAL-ADMIN-LINE, with user-identifying tokens removed
			line:  `[2016-11-10 19:10:49.230930] [22560|adminbox--04ec81f3361370d7f.northwest.stripe.io/kUku-rmgfZ-349 0000000000000000>a020f53ed1dd83ef] CANONICAL-ADMIN-LINE: path="/fonts/glyphicons-halflings-regular.woff" http_method=GET referer="/css/bootstrap3.min.css" response_content_type="application/octet-stream" status=200`,
			level: CriticalPlus,
		},
		{
			name: "canonical-monster-line",
			// test for case-insensitivity
			line:  `[2016-11-10 19:10:49.230930] [22560|adminbox--04ec81f3361370d7f.northwest.stripe.io/kUku-rmgfZ-349 0000000000000000>a020f53ed1dd83ef] canonical-monster-line: path="/fonts/glyphicons-halflings-regular.woff" http_method=GET referer="/css/bootstrap3.min.css" response_content_type="application/octet-stream" status=200`,
			level: CriticalPlus,
		},
		{
			name:  "PlainOldLogLine",
			line:  `[2016-11-10 19:01:02.461489] [21515|adminbox--04ec81f3361370d7f.northwest.stripe.io/kUku-wvrZK-28 0000000000000000>93e612b5bd9b69eb] HTTP response headers: Content-Type="text/html;charset=utf-8" Content-Length="10879" Set`,
			level: SheddablePlus,
		},
		{
			name:  "PlainOldLogLineWithClevelUppercase",
			line:  `[2016-11-10 19:01:02.461489] [21515|adminbox--04ec81f3361370d7f.northwest.stripe.io/kUku-wvrZK-28 0000000000000000>93e612b5bd9b69eb] HTTP response headers: Content-Type="text/html;charset=utf-8" Content-Length="10879" Set [clevel: Critical]`,
			level: Critical,
		},
		{
			name:  "PlainOldLogLineWithClevelLowercase",
			line:  `[2016-11-10 19:01:02.461489] [21515|adminbox--04ec81f3361370d7f.northwest.stripe.io/kUku-wvrZK-28 0000000000000000>93e612b5bd9b69eb] HTTP response headers: Content-Type="text/html;charset=utf-8" Content-Length="10879" Set [clevel: critical]`,
			level: Critical,
		},
		{
			name:  "LogLineWithClevelChalk",
			line:  `[2016-11-10 20:02:01.932272] [24607|adminbox--04ec81f3361370d7f.northwest.stripe.io/kUku-WdiJA-3204 0000000000000000>831e61790017a475] Showed info for merchant: merchant=acct_xxxxxxxxxxxxxxxxx tier=tier0 clevel=criticalplus`,
			level: CriticalPlus,
		},
	}

	for i, tc := range cases {
		name := strconv.Itoa(i)
		if tc.name != "" {
			name = tc.name
		}
		t.Run(name, func(t *testing.T) {
			l := Criticality(tc.line)
			assert.Equal(t, tc.level.String(), l.String())
		})
	}
}

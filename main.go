package main

import (
	"github.com/stripe/unilog/filters"
	"github.com/stripe/unilog/logger"
)

func main() {
	tf := &filters.TimePrefixFilter{}
	// Register flags so they're picked up when u.Main() calls flag.Parse() (ugh)
	tf.AddFlags()

	u := &logger.Unilog{
		Filters: []logger.Filter{
			logger.Filter(&filters.AusterityFilter{}),
			logger.Filter(tf),
		},
	}
	u.Main()
}

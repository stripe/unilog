package main

import (
	"github.com/stripe/unilog/clevels"
	"github.com/stripe/unilog/filters"
	"github.com/stripe/unilog/logger"
)

func main() {
	go clevels.SendSystemAusterityLevel()

	u := &logger.Unilog{
		Filters: []logger.Filter{
			logger.Filter(filters.AusterityFilter),
		},
	}
	u.Main()
}

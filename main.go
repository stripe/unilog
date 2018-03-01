package main

import (
	"github.com/stripe/unilog/filters"
	"github.com/stripe/unilog/logger"
)

func main() {
	u := &logger.Unilog{
		Filters: []logger.Filter{
			logger.Filter(filters.AusterityFilter),
		},
	}
	u.Main()
}

package common

import "time"

type Date struct {
	Year  int
	Month time.Month
	Day   int
}

type JItem struct {
	Job  string
	Done bool
}

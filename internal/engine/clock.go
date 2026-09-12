package engine

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// Clock is the engine's only source of time, so a run can be reproduced.
type Clock func() time.Time

// SystemClock reads the wall clock in UTC.
func SystemClock() time.Time { return time.Now().UTC() }

func (c Clock) now() time.Time {
	if c == nil {
		return SystemClock()
	}
	return c().UTC()
}

func (c Clock) stamp() *timestamppb.Timestamp {
	return timestamppb.New(c.now())
}

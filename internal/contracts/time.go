package contracts

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// now is a variable so tests can pin it; the ledger records real time otherwise.
var now = time.Now

func nowTimestamp() *timestamppb.Timestamp {
	return timestamppb.New(now().UTC())
}

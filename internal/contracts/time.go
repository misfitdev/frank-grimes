package contracts

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// Now is a variable so a caller can pin it; the ledger records real time
// otherwise. Transition and Observe stamp finding history through it, so a
// reproducible ledger requires setting it.
var Now = time.Now

func nowTimestamp() *timestamppb.Timestamp {
	return timestamppb.New(Now().UTC())
}

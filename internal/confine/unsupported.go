//go:build !darwin && !linux

package confine

import (
	"fmt"
	"runtime"
)

// platformDefault fails closed. An operator on a platform with no backend can
// supply a wrapper or accept an unconfined run, both explicitly.
func platformDefault() (Mechanism, error) {
	return nil, fmt.Errorf("confine: no built-in confinement for %s", runtime.GOOS)
}

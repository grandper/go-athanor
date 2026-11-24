//go:build !unix && !windows

package fixture

import "errors"

// errMulticastUnsupported is returned where the multicast interface of a socket cannot be set.
var errMulticastUnsupported = errors.New("setting the multicast interface is not supported on this platform")

// setMulticastInterface fails: this platform has no socket option for the multicast interface.
func setMulticastInterface(uintptr, [4]byte) error {
	return errMulticastUnsupported
}

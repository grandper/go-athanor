//go:build unix

package fixture

import "syscall"

// setMulticastInterface makes the socket send its multicast datagrams through the interface holding ip.
func setMulticastInterface(fd uintptr, ip [4]byte) error {
	return syscall.SetsockoptInet4Addr(int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, ip)
}

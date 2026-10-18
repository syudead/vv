package domain

import (
	"errors"
	"net"
	"strconv"
)

// ErrListenFailed leaves saved settings unchanged and restores the previous listener address.
var ErrListenFailed = errors.New("cannot listen on the new address")

type NetworkSettings struct {
	LANAccess bool
	Port      int
	// Addresses contains URLs for active non-loopback IPv4 interfaces only when LANAccess is enabled.
	Addresses []string
}

func LANListenAddr(lanAccess bool, port int) string {
	host := "127.0.0.1"
	if lanAccess {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

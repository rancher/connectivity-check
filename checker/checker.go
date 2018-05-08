package checker

import (
	"github.com/rancher/go-rancher-metadata/metadata"
)

const (
	// DefaultCheckInterval ...
	DefaultCheckInterval = 5000

	// DefaultPeerConnectionTimeoutInterval ...
	DefaultPeerConnectionTimeoutInterval = 1000
)

// ConnectivityChecker interface specifies the methods available
type ConnectivityChecker interface {
	Ok() bool
	Update(ip string)
}

// New returns a new instance of ConnectivityChecker
func New(port, syncInterval, connectTimeout int, mc metadata.Client) (*PeersWatcher, error) {
	return NewPeersWatcher(port, syncInterval, connectTimeout, mc)
}

package checker

import (
	"github.com/rancher/go-rancher-metadata/metadata"
)

const (
	// DefaultCheckInterval ...
	DefaultCheckInterval = 5000
)

// ConnectivityChecker interface specifies the methods available
type ConnectivityChecker interface {
	Ok() bool
	Update(ip string)
}

// New returns a new instance of ConnectivityChecker
func New(port, syncInterval int, mc metadata.Client) (*PeersWatcher, error) {
	return NewPeersWatcher(port, syncInterval, mc)
}

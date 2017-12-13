package checker

import (
	"testing"

	"github.com/leodotcloud/log"
	"github.com/rancher/go-rancher-metadata/metadata"
)

// Some of the tests can run only when in development,
// remember to disable this before commiting the code.
const inDevelopment = false

func TestServer(t *testing.T) {
	if !inDevelopment {
		return
	}
	log.SetLevelString("debug")

	log.Debugf("starting")
	mc, err := metadata.NewClientAndWait("http://169.254.169.250/latest")
	if err != nil {
		log.Errorf("error creating metadata client: %v", err)
		t.Fail()
	}
	hc, _ := New(80, 10, mc)
	s, err := NewServer(9090, hc)
	if err != nil {
		log.Errorf("couldn't create server: %v", err)
		t.Fail()
	}

	s.Run()
}

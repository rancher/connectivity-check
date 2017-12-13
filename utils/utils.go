package utils

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
)

// IsReachable checks if the given IP address responds
// to given URL request and the response has right values
func IsReachable(url, result string) (bool, error) {
	logrus.Debugf("is %v Reachable", url)

	timeout := time.Duration(1 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}

	resp, err := client.Get(url)
	if err != nil {
		return false, err
	}

	logrus.Debugf("resp: %+v", resp)

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("got StatusCode: %v", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	if string(body) != result {
		return false, fmt.Errorf("response from peer: %v didn't match expected: %v", string(body), result)
	}

	resp.Body.Close()
	return true, nil
}

// IsValidPort checks if the input port string is valid.
// Valid port range : 1025 - 65535
func IsValidPort(port int) bool {
	if port < 1024 && 65535 < port {
		return false
	}

	return true
}

package checker

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/rancher/log"
)

const (
	// DefaultServerPort ...
	DefaultServerPort = 80
)

// Server ...
type Server struct {
	port int
	cc   ConnectivityChecker
	l    net.Listener
}

// NewServer ...
func NewServer(port int, cc ConnectivityChecker) (*Server, error) {
	return &Server{
		port: port,
		cc:   cc,
	}, nil
}

// GetPort ...
func (s *Server) GetPort() int {
	return s.port
}

// Shutdown ...
func (s *Server) Shutdown() error {
	log.Infof("Shutting down server")
	return s.l.Close()
}

// Run ...
func (s *Server) Run() error {
	log.Infof("Starting webserver on port: %v", s.port)
	http.HandleFunc("/ping", s.pingHandler)
	http.HandleFunc("/connectivity", s.connectivityHandler)

	l, err := net.Listen("tcp", fmt.Sprintf(":%v", s.port))
	if err != nil {
		log.Errorf("error listening: %v", err)
		return err
	}
	s.l = l
	go http.Serve(l, nil)

	return nil
}

func getSourceIP(r *http.Request) string {
	reqIP := ""
	splits := strings.Split(r.RemoteAddr, ":")
	if len(splits) == 2 {
		reqIP = splits[0]
	}
	return reqIP
}

func (s *Server) pingHandler(w http.ResponseWriter, r *http.Request) {
	reqIP := getSourceIP(r)
	s.cc.Update(reqIP)
	fmt.Fprintf(w, "pong")
}

func (s *Server) connectivityHandler(w http.ResponseWriter, r *http.Request) {
	if s.cc.Ok() {
		w.Write([]byte("OK"))
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("NOT OK"))
	}
}

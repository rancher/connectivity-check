package checker

import (
	"sync"
	"time"

	"github.com/rancher/go-rancher-metadata/metadata"
	"github.com/rancher/log"
)

const (
	connectivityCheckServiceName = "connectivity-check"
)

type PeersWatcher struct {
	sync.Mutex
	ok                    bool
	s                     *Server
	mc                    metadata.Client
	peers                 map[string]*Peer
	peersMapByIP          map[string]*Peer
	exit                  chan bool
	peerCheckInterval     int
	peerConnectionTimeout int
}

type mdInfo struct {
	ipsecState        string
	connCheckState    string
	hostsMap          map[string]*metadata.Host
	peerContainersMap map[string]*metadata.Container
	ccContainersMap   map[string]*metadata.Container
}

func NewPeersWatcher(port, peerCheckInterval, peerConnectionTimeout int, mc metadata.Client) (*PeersWatcher, error) {
	log.Debugf("creating new PeersWatcher with port=%v, peerCheckInterval=%v", port, peerCheckInterval)

	pw := &PeersWatcher{mc: mc,
		peerCheckInterval:     peerCheckInterval,
		peerConnectionTimeout: peerConnectionTimeout,
		exit: make(chan bool),
	}
	s, err := NewServer(port, pw)
	if err != nil {
		log.Errorf("error creating server: %v", err)
		return nil, err
	}
	pw.s = s
	return pw, nil
}

func getInfoFromMetadata(mc metadata.Client) (*mdInfo, error) {
	mdInfo := &mdInfo{
		hostsMap:          make(map[string]*metadata.Host),
		peerContainersMap: make(map[string]*metadata.Container),
		ccContainersMap:   make(map[string]*metadata.Container),
	}

	selfHost, err := mc.GetSelfHost()
	if err != nil {
		log.Errorf("error fetching self host from metadata: %v", err)
		return mdInfo, err
	}

	hosts, err := mc.GetHosts()
	if err != nil {
		log.Errorf("error fetching peers from metadata: %v", err)
		return mdInfo, err
	}

	for index, aHost := range hosts {
		if aHost.UUID == selfHost.UUID {
			continue
		}
		mdInfo.hostsMap[aHost.UUID] = &hosts[index]
	}

	selfService, err := mc.GetSelfService()
	if err != nil {
		log.Errorf("error fetching self service from metadata: %v", err)
		return mdInfo, err
	}
	mdInfo.ipsecState = selfService.State

	for index, aPeer := range selfService.Containers {
		if aPeer.HostUUID == selfHost.UUID {
			continue
		}
		mdInfo.peerContainersMap[aPeer.UUID] = &selfService.Containers[index]
	}

	services, err := mc.GetServices()
	if err != nil {
		log.Errorf("error fetching services from metadata: %v", err)
		return mdInfo, err
	}
	for _, aService := range services {
		if aService.Name != connectivityCheckServiceName {
			continue
		}
		mdInfo.connCheckState = aService.State
		for index, c := range aService.Containers {
			if c.HostUUID == selfHost.UUID {
				continue
			}
			mdInfo.ccContainersMap[c.HostUUID] = &aService.Containers[index]
		}
	}

	return mdInfo, err
}

func (pw *PeersWatcher) doWork() {
	pw.Lock()
	defer pw.Unlock()
	log.Debugf("PeersWatcher: doWork: start")

	// Get peers info from metadata
	mdInfo, err := getInfoFromMetadata(pw.mc)
	if err != nil {
		log.Errorf("error fetching hostsMap: %v", err)
	}
	log.Debugf("hostsMap: %v", mdInfo.hostsMap)
	log.Debugf("peerContainersMap: %v", mdInfo.peerContainersMap)
	log.Debugf("ccContainersMap: %v", mdInfo.ccContainersMap)

	newPeersMap := make(map[string]*Peer)
	newPeersMapByIP := make(map[string]*Peer)
	// Update or create peers
	for uuid, aPeerContainer := range mdInfo.peerContainersMap {
		aPeer, found := pw.peers[uuid]
		if found {
			//log.Debugf("updating peer: %v", *aPeerContainer)
			aPeer.Lock()
			aPeer.container = aPeerContainer
			aPeer.ccContainer = mdInfo.ccContainersMap[aPeerContainer.HostUUID]
			aPeer.host = mdInfo.hostsMap[aPeerContainer.HostUUID]
			aPeer.Unlock()
			newPeersMap[uuid] = aPeer
			newPeersMapByIP[aPeerContainer.PrimaryIp] = aPeer
			delete(pw.peers, uuid)
		} else {
			host, ok := mdInfo.hostsMap[aPeerContainer.HostUUID]
			if !ok || host == nil {
				log.Infof("for new peer container: %v, host info is not available yet in metadata", *aPeerContainer)
				continue
			}
			log.Infof("new peer container: %v", *aPeerContainer)
			aPeer = &Peer{
				uuid:              uuid,
				container:         aPeerContainer,
				ccContainer:       mdInfo.ccContainersMap[aPeerContainer.HostUUID],
				host:              host,
				checkInterval:     pw.peerCheckInterval,
				connectionTimeout: pw.peerConnectionTimeout,
				exit:              make(chan bool),
			}
			newPeersMap[uuid] = aPeer
			newPeersMapByIP[aPeerContainer.PrimaryIp] = aPeer
			aPeer.Start()
		}
	}

	// Delete peers
	for uuid, aPeer := range pw.peers {
		log.Infof("peer container deleted: %v", *(aPeer.container))
		aPeer.Shutdown()
		delete(pw.peers, uuid)
	}

	pw.peers = newPeersMap
	pw.peersMapByIP = newPeersMapByIP

	// Figure out current connectivity state
	ok := true
	if shouldConsider(mdInfo) {
		for peerIP, peer := range pw.peersMapByIP {
			if !peer.Consider() {
				log.Debugf("Peer(%v): not considered for connectivity state", peer.uuid)
				continue
			}
			log.Debugf("peer(%v): %+v", peerIP, peer)
			if peer.count == 0 {
				ok = false
				log.Debugf("peer: %v is not reachable", peerIP)
			}
		}
	} else {
		log.Debugf("PeersWatcher: skipping actual peers state ipsecState=%v connCheckState=%v",
			mdInfo.ipsecState, mdInfo.connCheckState)
	}
	pw.ok = ok

	log.Debugf("PeersWatcher: current connectivity state=%v", pw.ok)
	log.Debugf("PeersWatcher: doWork: end")

}

func (pw *PeersWatcher) Run() {
	for {
		select {
		case _, ok := <-pw.exit:
			if !ok {
				log.Infof("PeersWatcher: stopped")
				return
			}
		default:
			pw.doWork()
		}

		time.Sleep(time.Duration(pw.peerCheckInterval) * time.Millisecond)
	}
}

func (pw *PeersWatcher) Start() error {
	log.Debugf("PeersWatcher: Start")
	go pw.Run()
	go pw.s.Run()
	return nil
}

func (pw *PeersWatcher) Ok() bool {
	pw.Lock()
	defer pw.Unlock()
	return pw.ok
}

func (pw *PeersWatcher) Update(peerIP string) {
	log.Debugf("PeersWatcher: update status for %v", peerIP)
	pw.Lock()
	defer pw.Unlock()
	peer, found := pw.peersMapByIP[peerIP]
	if found {
		peer.UpdateSuccess()
	}
}

func (pw *PeersWatcher) Shutdown() error {
	log.Infof("PeersWatcher: shutdown")
	if err := pw.s.Shutdown(); err != nil {
		log.Errorf("error shutting down server: %v", err)
		return err
	}

	close(pw.exit)

	return nil
}

func shouldConsider(mdInfo *mdInfo) bool {
	consider := false
	if mdInfo.ipsecState == "active" &&
		mdInfo.connCheckState == "active" {
		consider = true
	}
	return consider
}

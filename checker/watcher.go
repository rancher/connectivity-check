package checker

import (
	"sync"
	"time"

	"github.com/leodotcloud/log"
	"github.com/rancher/go-rancher-metadata/metadata"
)

type PeersWatcher struct {
	sync.Mutex
	ok                bool
	s                 *Server
	mc                metadata.Client
	peers             map[string]*Peer
	peersMapByIP      map[string]*Peer
	exit              chan bool
	peerCheckInterval int
}

func NewPeersWatcher(port, peerCheckInterval int, mc metadata.Client) (*PeersWatcher, error) {
	log.Debugf("creating new PeersWatcher with port=%v, peerCheckInterval=%v", port, peerCheckInterval)

	pw := &PeersWatcher{mc: mc,
		peerCheckInterval: peerCheckInterval,
		exit:              make(chan bool),
	}
	s, err := NewServer(port, pw)
	if err != nil {
		log.Errorf("error creating server: %v", err)
		return nil, err
	}
	pw.s = s
	return pw, nil
}

func getInfoFromMetadata(mc metadata.Client) (
	map[string]*metadata.Host,
	map[string]*metadata.Container,
	error) {
	hostsMap := make(map[string]*metadata.Host)
	peerContainersMap := make(map[string]*metadata.Container)

	selfHost, err := mc.GetSelfHost()
	if err != nil {
		log.Errorf("error fetching self host from metadata: %v", err)
		return hostsMap, peerContainersMap, err
	}

	hosts, err := mc.GetHosts()
	if err != nil {
		log.Errorf("error fetching peers from metadata: %v", err)
		return hostsMap, peerContainersMap, err
	}

	for index, aHost := range hosts {
		if aHost.UUID == selfHost.UUID {
			continue
		}
		hostsMap[aHost.UUID] = &hosts[index]
	}

	selfService, err := mc.GetSelfService()
	if err != nil {
		log.Errorf("error fetching self service from metadata: %v", err)
		return hostsMap, peerContainersMap, err
	}

	for index, aPeer := range selfService.Containers {
		if aPeer.HostUUID == selfHost.UUID {
			continue
		}
		peerContainersMap[aPeer.UUID] = &selfService.Containers[index]
	}
	return hostsMap, peerContainersMap, nil
}

func (pw *PeersWatcher) doWork() {
	pw.Lock()
	defer pw.Unlock()
	log.Debugf("PeersWatcher: doWork: start")

	// Get peers
	hostsMap, peerContainersMap, err := getInfoFromMetadata(pw.mc)
	if err != nil {
		log.Errorf("error fetching hostsMap: %v", err)
	}
	log.Debugf("hostsMap: %v", hostsMap)
	log.Debugf("peerContainersMap: %v", peerContainersMap)

	newPeersMap := make(map[string]*Peer)
	newPeersMapByIP := make(map[string]*Peer)
	// Update or create peers
	for uuid, aPeerContainer := range peerContainersMap {
		aPeer, found := pw.peers[uuid]
		if found {
			//log.Debugf("updating peer: %v", *aPeerContainer)
			aPeer.Lock()
			aPeer.container = aPeerContainer
			aPeer.host = hostsMap[aPeerContainer.HostUUID]
			aPeer.Unlock()
			newPeersMap[uuid] = aPeer
			newPeersMapByIP[aPeerContainer.PrimaryIp] = aPeer
			delete(pw.peers, uuid)
		} else {
			log.Infof("new peer container: %v", *aPeerContainer)
			aPeer = &Peer{
				uuid:          uuid,
				container:     aPeerContainer,
				host:          hostsMap[aPeerContainer.HostUUID],
				checkInterval: pw.peerCheckInterval,
				exit:          make(chan bool),
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
	pw.ok = ok

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

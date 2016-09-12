package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/markstgodard/tick/registry"
)

var random = rand.New(rand.NewSource(time.Now().UnixNano()))

const interval = 5 * time.Second

var heartbeater *heartbeat

type Peer struct {
	AppName string
	Address string
}

type heartbeat struct {
	sync.RWMutex
	AppName      string
	IP           string
	RegistryHost string
	interval     time.Duration
	Peer         Peer
	Polling      bool
	doneChan     chan chan struct{}
}

func newHeartbeat(interval time.Duration, registryHost, ip, appName string) *heartbeat {
	return &heartbeat{
		AppName:      appName,
		IP:           ip,
		RegistryHost: registryHost,
		interval:     interval,
		doneChan:     make(chan chan struct{}),
	}
}

func (h *heartbeat) Start() {
	ticker := time.NewTicker(h.interval)

	for {
		select {
		case <-ticker.C:
			h.Send()
			h.FindPeer()
		case stopped := <-h.doneChan:
			ticker.Stop()
			close(stopped)
			return
		}
	}
}

func (h *heartbeat) PingPeers() {
	ticker := time.NewTicker(1 * time.Second)

	for {
		select {
		case <-ticker.C:
			// only start pinging if has peers
			if h.Polling && h.Peer.Address != "" {
				h.Ping()
			}
		case stopped := <-h.doneChan:
			ticker.Stop()
			close(stopped)
			return
		}
	}
}

func (h *heartbeat) Ping() {
	start := time.Now()
	resp, err := http.Get(fmt.Sprintf("http://%s", h.Peer.Address))
	if err != nil {
		fmt.Printf("could not talk to peer: %s\n", h.Peer.Address)
		return
	}
	defer resp.Body.Close()

	ttp := time.Since(start)

	fmt.Printf("TTP: [%v] source [%s] target [%s]\n", ttp, h.IP, h.Peer.Address)
}

func (h *heartbeat) Send() error {
	url := fmt.Sprintf("http://%s/api/v1/instances", h.RegistryHost)

	tags := []string{}
	if h.Peer.Address != "" {
		tags = append(tags, fmt.Sprintf("%s=%s", h.Peer.AppName, h.Peer.Address))
	}

	s := registry.ServiceInstance{
		ServiceName: fmt.Sprintf("%s/%s", h.AppName, os.Getenv("CF_INSTANCE_INDEX")),
		Endpoint: registry.ServiceEndpoint{
			Type:  "tcp",
			Value: fmt.Sprintf("%s:%d", h.IP, 8080),
		},
		Status: "UP",
		TTL:    10,
		Tags:   tags,
	}

	jsonStr, err := json.Marshal(s)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("Could not register app, resp code [%d]\n", resp.StatusCode)
	}

	fmt.Printf("[%s/%s] registered ip [%s]\n", h.AppName, os.Getenv("CF_INSTANCE_INDEX"), h.IP)
	return nil
}

func (h *heartbeat) FindPeer() {
	url := fmt.Sprintf("http://%s/api/v1/instances", h.RegistryHost)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("could not talk to %s\n", url)
		return
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var instances registry.Instances
	err = json.Unmarshal(body, &instances)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	// get a random peer (exclude self)
	total := len(instances.ServiceInstances)
	randIndices := random.Perm(total)
	for i := 0; i < total; i++ {
		randIdx := randIndices[i]
		otherApp := instances.ServiceInstances[randIdx].ServiceName
		otherIP := instances.ServiceInstances[randIdx].Endpoint.Value
		if !strings.HasPrefix(otherIP, h.IP) {
			h.Peer = Peer{
				AppName: otherApp,
				Address: otherIP,
			}
		}
	}
}

func getOverlayAddr() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	var overlayIP string
	for _, addr := range addrs {
		listenAddr := strings.Split(addr.String(), "/")[0]
		if strings.HasPrefix(listenAddr, "10.255.") {
			overlayIP = listenAddr
		}
	}
	return overlayIP
}

func getAppName() string {
	vcap := os.Getenv("VCAP_APPLICATION")
	if vcap == "" {
		panic("Missing VCAP_APPLICATION env variable")
	}

	type vcapApp struct {
		ApplciationName string `json:"application_name"`
	}

	var va vcapApp
	err := json.Unmarshal([]byte(vcap), &va)
	if err != nil {
		panic("Error invalid VCAP_APPLICATION json format")
	}

	return va.ApplciationName
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	host := os.Getenv("REGISTRY_HOST")
	if host == "" {
		panic("Missing REGISTRY_HOST env variable")
	}

	// hack: use instance ip if overlay not present
	ip := getOverlayAddr()
	if ip == "" {
		ip = os.Getenv("CF_INSTANCE_IP")
	}

	heartbeater = newHeartbeat(interval, host, ip, getAppName())
	go heartbeater.Start()
	go heartbeater.PingPeers()

	http.HandleFunc("/", index)
	http.HandleFunc("/poll", poll)
	http.HandleFunc("/access", access)
	log.Fatal(http.ListenAndServe(":"+os.Getenv("PORT"), nil))
}

func index(w http.ResponseWriter, r *http.Request) {
	s := fmt.Sprintf("app: %s ip:%s:%d peer:%s\n", heartbeater.AppName, heartbeater.IP, 8080, heartbeater.Peer)
	fmt.Printf(s)
	fmt.Fprintf(w, s)
}

// this may not work since we are round robinning and if there are multiple app instances
// maybe put in the service registry itself
func access(w http.ResponseWriter, r *http.Request) {
	otherApp := strings.Split(heartbeater.Peer.AppName, "/")[0]
	s := fmt.Sprintf("cf access-allow %s %s --protocol tcp --port 8080", heartbeater.AppName, otherApp)
	fmt.Fprintf(w, s)
}

// post to turn on, delete to turn off
func poll(w http.ResponseWriter, r *http.Request) {
	heartbeater.Lock()
	defer heartbeater.Unlock()

	if r.Method == http.MethodPost {
		heartbeater.Polling = true
		w.WriteHeader(http.StatusOK)
	}
	if r.Method == http.MethodDelete {
		heartbeater.Polling = false
		w.WriteHeader(http.StatusOK)
	}
	w.WriteHeader(http.StatusInternalServerError)
}

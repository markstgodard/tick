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
	"time"
)

var random = rand.New(rand.NewSource(time.Now().UnixNano()))

const interval = 10 * time.Second

var ip, host string

type Peer struct {
	Address string
}

type Instances struct {
	ServiceInstances []serviceInstance `json:"instances"`
}

type serviceInstance struct {
	ServiceName string          `json:"service_name"`
	Endpoint    serviceEndpoint `json:"endpoint"`
	Status      string          `json:"status,omitempty"`
	TTL         int             `json:"ttl,omitempty"`
	Tags        []string
}

type serviceEndpoint struct {
	// type: "http", "tcp"
	Type string `json:"type"`
	// e.g. "172.135.10.1:8080" or "http://myapp.bosh-lite.com".
	Value string `json:"value"`
}

type heartbeat struct {
	Host     string
	interval time.Duration
	Peer     Peer
	doneChan chan chan struct{}
}

func newHeartbeat(interval time.Duration, registryHost string) *heartbeat {
	return &heartbeat{
		Host:     registryHost,
		interval: interval,
		doneChan: make(chan chan struct{}),
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

func (h *heartbeat) Send() error {
	url := fmt.Sprintf("http://%s/api/v1/instances", h.Host)
	fmt.Println("url:", url)

	tags := []string{}
	if h.Peer.Address != "" {
		tags = append(tags, fmt.Sprintf("peer=%s", h.Peer.Address))
	}

	s := serviceInstance{
		ServiceName: fmt.Sprintf("%s/%s", os.Getenv("CF_INSTANCE_GUID"), os.Getenv("CF_INSTANCE_INDEX")),
		Endpoint: serviceEndpoint{
			Type:  "tcp",
			Value: fmt.Sprintf("%s:%d", ip, 8080),
		},
		Status: "UP",
		TTL:    20,
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

	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))

	return nil
}

func (h *heartbeat) FindPeer() {
	url := fmt.Sprintf("http://%s/api/v1/instances", host)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("could not talk to %s\n", url)
		return
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var instances Instances
	err = json.Unmarshal(body, &instances)
	if err != nil {
		fmt.Println("error:", err)
	}
	fmt.Printf("%+v", instances)

	// get a random peer (exclude self)
	total := len(instances.ServiceInstances)
	randIndices := random.Perm(total)
	for i := 0; i < total; i++ {
		randIdx := randIndices[i]
		otherIP := instances.ServiceInstances[randIdx].Endpoint.Value
		if !strings.HasPrefix(otherIP, ip) {
			h.Peer = Peer{
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

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	host = os.Getenv("REGISTRY_HOST")
	if host == "" {
		panic("Missing REGISTRY_HOST env variable")
	}

	// hack: use instance ip if overlay not present
	ip = getOverlayAddr()
	if ip == "" {
		ip = os.Getenv("CF_INSTANCE_IP")
	}

	heartbeat := newHeartbeat(interval, host)
	go heartbeat.Start()

	http.HandleFunc("/", index)
	log.Fatal(http.ListenAndServe(":"+os.Getenv("PORT"), nil))
}

func index(w http.ResponseWriter, r *http.Request) {
	s := fmt.Sprintf("endpoint: %s:%d\n", ip, 8080)
	fmt.Printf(s)
	fmt.Fprintf(w, s)
}

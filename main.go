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

	"github.com/markstgodard/tick/registry"
)

var random = rand.New(rand.NewSource(time.Now().UnixNano()))

const interval = 5 * time.Second

var heartbeater *heartbeat

type heartbeat struct {
	AppName      string
	IP           string
	RegistryHost string
	interval     time.Duration
	Peer         string
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

func (h *heartbeat) Send() error {
	url := fmt.Sprintf("http://%s/api/v1/instances", h.RegistryHost)
	fmt.Println("url:", url)

	tags := []string{}
	if h.Peer != "" {
		tags = append(tags, fmt.Sprintf("peer=%s", h.Peer))
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

	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))

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
	fmt.Printf("%+v", instances)

	// get a random peer (exclude self)
	total := len(instances.ServiceInstances)
	randIndices := random.Perm(total)
	for i := 0; i < total; i++ {
		randIdx := randIndices[i]
		otherIP := instances.ServiceInstances[randIdx].Endpoint.Value
		if !strings.HasPrefix(otherIP, h.IP) {
			h.Peer = otherIP
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

	http.HandleFunc("/", index)
	log.Fatal(http.ListenAndServe(":"+os.Getenv("PORT"), nil))
}

func index(w http.ResponseWriter, r *http.Request) {
	s := fmt.Sprintf("app: %s ip:%s:%d peer:%s\n", heartbeater.AppName, heartbeater.IP, 8080, heartbeater.Peer)
	fmt.Printf(s)
	fmt.Fprintf(w, s)
}

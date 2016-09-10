package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

const interval = 10 * time.Second

type serviceInstance struct {
	ServiceName string          `json:"service_name"`
	Endpoint    serviceEndpoint `json:"endpoint"`
	Status      string          `json:"status,omitempty"`
	TTL         int             `json:"ttl,omitempty"`
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

	s := serviceInstance{
		ServiceName: fmt.Sprintf("%s/%s", os.Getenv("CF_INSTANCE_GUID"), os.Getenv("CF_INSTANCE_INDEX")),
		Endpoint: serviceEndpoint{
			Type:  "tcp",
			Value: fmt.Sprintf("%s:%s", os.Getenv("CF_INSTANCE_IP"), os.Getenv("CF_INSTANCE_PORT")),
		},
		Status: "UP",
		TTL:    60,
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

	fmt.Println("response Status:", resp.Status)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))

	return nil
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

	heartbeat := newHeartbeat(interval, host)
	go heartbeat.Start()

	http.HandleFunc("/", index)
	log.Fatal(http.ListenAndServe(":"+os.Getenv("PORT"), nil))
}

func index(w http.ResponseWriter, r *http.Request) {
	s := fmt.Sprintf("endpoint: %s:%s\n", os.Getenv("CF_INSTANCE_IP"), os.Getenv("CF_INSTANCE_PORT"))
	fmt.Printf(s)
	fmt.Fprintf(w, s)
}

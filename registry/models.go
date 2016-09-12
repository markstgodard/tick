package registry

type Instances struct {
	ServiceInstances []ServiceInstance `json:"instances"`
}

type ServiceInstance struct {
	ServiceName string          `json:"service_name"`
	Endpoint    ServiceEndpoint `json:"endpoint"`
	Status      string          `json:"status,omitempty"`
	TTL         int             `json:"ttl,omitempty"`
	Tags        []string
}

type ServiceEndpoint struct {
	// type: "http", "tcp"
	Type string `json:"type"`
	// e.g. "172.135.10.1:8080" or "http://myapp.bosh-lite.com".
	Value string `json:"value"`
}

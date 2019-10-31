package envoy

import (
	"time"
)

// Upstream is a compact form of an Envoy cluster and virtual host
type Upstream struct {
	Name     string        `json:"name"`
	Host     string        `json:"host"`
	Port     uint32        `json:"port"`
	PortName string        `json:"portName"`
	Domains  []string      `json:"domains"`
	Prefix   string        `json:"prefix"`
	Retries  uint32        `json:"retries"`
	Timeout  time.Duration `json:"timeout"`
	Canary   *Canary       `json:"canary"`
}

// Canary is a compact form of an Envoy weighted cluster
type Canary struct {
	PrimaryCluster string `json:"primaryCluster"`
	CanaryCluster  string `json:"canaryCluster"`
	CanaryWeight   int    `json:"canaryWeight"`
}

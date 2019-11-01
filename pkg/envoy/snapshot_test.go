package envoy

import (
	"fmt"
	"testing"
	"time"
)

func mockUpstream(i int, prefix string) (string, Upstream) {
	u := Upstream{
		Name:     fmt.Sprintf("app%d-test-9898", i),
		Host:     fmt.Sprintf("app%d.test", i),
		Port:     9898,
		PortName: "http",
		Domains:  []string{fmt.Sprintf("app%d.test.io", i)},
		Prefix:   prefix,
		Retries:  2,
		Timeout:  2 * time.Second,
		Canary: &Canary{
			PrimaryCluster: fmt.Sprintf("app%d-primary-test-9898", i),
			CanaryCluster:  fmt.Sprintf("app%d-canary-test-9898", i),
			CanaryWeight:   50,
		},
	}
	return fmt.Sprintf("test/app%d", i), u
}

func mockUpstreams(prefix string) map[string]Upstream {
	m := make(map[string]Upstream)
	for i := 0; i < 10; i++ {
		k, u := mockUpstream(i, prefix)
		m[k] = u
	}
	return m
}

func TestSnapshot_Sync(t *testing.T) {
	cache := NewCache(true)
	snapshot := NewSnapshot(cache)
	snapshot.nodeId = "test"

	// test init
	i := 0
	for key, value := range mockUpstreams("/") {
		snapshot.Store(key, value)
		i++
		if i == 5 {
			break
		}
	}

	err := snapshot.Sync()
	if err != nil {
		t.Fatal(err.Error())
	}

	snap, err := snapshot.cache.GetSnapshot(snapshot.nodeId)
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(snap.Clusters.Items) != 5 {
		t.Errorf("Got clusters %v wanted %v", len(snap.Clusters.Items), 5)
	}

	if snap.Listeners.Version != "1" {
		t.Errorf("Got version %v wanted %v", snap.Listeners.Version, "1")
	}

	// test insert
	for key, value := range mockUpstreams("/") {
		snapshot.Store(key, value)
	}

	err = snapshot.Sync()
	if err != nil {
		t.Fatal(err.Error())
	}

	snap, err = snapshot.cache.GetSnapshot(snapshot.nodeId)
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(snap.Clusters.Items) != 10 {
		t.Errorf("Got clusters %v wanted %v", len(snap.Clusters.Items), 10)
	}

	if snap.Listeners.Version != "2" {
		t.Errorf("Got version %v wanted %v", snap.Listeners.Version, "2")
	}

	// test update
	for i := 0; i < 2; i++ {
		k, u := mockUpstream(i, "/test")
		snapshot.Store(k, u)
	}

	err = snapshot.Sync()
	if err != nil {
		t.Fatal(err.Error())
	}

	snap, err = snapshot.cache.GetSnapshot(snapshot.nodeId)
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(snap.Clusters.Items) != 10 {
		t.Errorf("Got clusters %v wanted %v", len(snap.Clusters.Items), 10)
	}

	if snap.Listeners.Version != "3" {
		t.Errorf("Got version %v wanted %v", snap.Listeners.Version, "3")
	}

	// test delete
	for i := 0; i < 10; i++ {
		snapshot.Delete(fmt.Sprintf("test/app%d", i))
	}

	err = snapshot.Sync()
	if err != nil {
		t.Fatal(err.Error())
	}

	snap, err = snapshot.cache.GetSnapshot(snapshot.nodeId)
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(snap.Clusters.Items) != 0 {
		t.Errorf("Got clusters %v wanted %v", len(snap.Clusters.Items), 0)
	}

	if snap.Listeners.Version != "4" {
		t.Errorf("Got version %v wanted %v", snap.Listeners.Version, "4")
	}
}

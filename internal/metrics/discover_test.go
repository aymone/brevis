package metrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	k8s "brevis.sh/brevis/internal/execution/kubernetes"
)

func pod(name, ip string, ready bool, ann map[string]string) k8s.Pod {
	st := "False"
	if ready {
		st = "True"
	}
	return k8s.Pod{
		Metadata: k8s.Metadata{Name: name, Annotations: ann},
		Status:   &k8s.PodStatus{Phase: "Running", PodIP: ip, Conditions: []k8s.Condition{{Type: "Ready", Status: st}}},
		// The ports the cases below ask for are declared; the undeclared ones
		// have their own pod.
		Spec: k8s.PodSpec{Containers: []k8s.Container{{Name: "c", Ports: []k8s.ContainerPort{{ContainerPort: 9090}, {ContainerPort: 8081}}}}},
	}
}

func gw(owner string, extra ...string) map[string]string {
	a := map[string]string{AnnotationKind: "gateway", AnnotationName: owner}
	for i := 0; i+1 < len(extra); i += 2 {
		a[extra[i]] = extra[i+1]
	}
	return a
}

func TestTargetsFromPods(t *testing.T) {
	notRunning := pod("p-pending", "10.0.0.9", true, gw("g"))
	notRunning.Status.Phase = "Pending"
	terminating := pod("p-term", "10.0.0.8", true, gw("g"))
	terminating.Metadata.DeletionTimestamp = "2026-01-01T00:00:00Z"
	noStatus := k8s.Pod{Metadata: k8s.Metadata{Name: "p-nostatus", Annotations: gw("g")}}
	hostNet := pod("host-net", "10.0.0.7", true, gw("g"))
	hostNet.Spec.HostNetwork = true
	sameAsHost := pod("same-as-host", "10.0.0.7", true, gw("g"))
	sameAsHost.Status.HostIP = "10.0.0.7"
	pods := []k8s.Pod{
		pod("b", "10.0.0.2", true, gw("zarv_ingestion")),
		pod("a", "10.0.0.1", true, gw("zarv_ingestion")),
		pod("api", "10.0.1.1", true, map[string]string{AnnotationKind: "endpoints", AnnotationName: "example-api", AnnotationPort: "8081", AnnotationPath: "/m"}),
		pod("not-ready", "10.0.0.3", false, gw("g")),
		pod("plain", "10.0.0.4", true, nil),
		pod("other-kind", "10.0.0.5", true, map[string]string{AnnotationKind: "database", AnnotationName: "x"}),
		pod("no-name", "10.0.0.6", true, map[string]string{AnnotationKind: "gateway"}),
		pod("bad-port", "10.0.0.7", true, gw("g", AnnotationPort, "70000")),
		pod("word-port", "10.0.0.7", true, gw("g", AnnotationPort, "http")),
		pod("path-host", "10.0.0.7", true, gw("g", AnnotationPath, "//evil/metrics")),
		pod("path-query", "10.0.0.7", true, gw("g", AnnotationPath, "/metrics?x=1")),
		pod("path-relative", "10.0.0.7", true, gw("g", AnnotationPath, "metrics")),
		pod("undeclared-port", "10.0.0.7", true, gw("g", AnnotationPort, "6379")),
		hostNet,
		sameAsHost,
		pod("loopback", "127.0.0.1", true, gw("g")),
		pod("metadata", "169.254.169.254", true, gw("g")),
		pod("no-ip", "", true, gw("g")),
		pod("junk-ip", "not-an-ip", true, gw("g")),
		notRunning, terminating, noStatus,
	}
	got := TargetsFromPods(pods)
	var urls []string
	for _, g := range got {
		urls = append(urls, g.String()+"="+g.url)
	}
	want := []string{
		"gateway:zarv_ingestion=http://10.0.0.1:9090/metrics", // by pod name: a, api, b
		"endpoints:example-api=http://10.0.1.1:8081/m",
		"gateway:zarv_ingestion=http://10.0.0.2:9090/metrics",
	}
	if strings.Join(urls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(urls, "\n"), strings.Join(want, "\n"))
	}
	if v6 := TargetsFromPods([]k8s.Pod{pod("v6", "fd00::1", true, gw("g"))}); len(v6) != 1 || v6[0].url != "http://[fd00::1]:9090/metrics" {
		t.Fatalf("%v", v6)
	}
}

func TestSetTargetsScaleDownDoesNotLeaveASilentReplica(t *testing.T) {
	c, ck := newTest()
	two := []Target{{Kind: Gateway, Owner: "g", url: "http://10.0.0.1:9090/metrics"}, {Kind: Gateway, Owner: "g", url: "http://10.0.0.2:9090/metrics"}}
	c.SetTargets(two)
	if !c.Configured(Gateway) || c.Configured(Endpoints) {
		t.Fatal("configured")
	}
	for _, tg := range c.targets {
		feed(c, ck, tg, []Sample{recv("s", 1)})
	}
	ck.t = ck.t.Add(5 * time.Second)
	for _, tg := range c.targets {
		feed(c, ck, tg, []Sample{recv("s", 3)})
	}
	before := c.Seq(Gateway)
	// The second replica is scaled down: it stops answering and is no longer announced.
	c.SetTargets(two[:1])
	if c.Seq(Gateway) == before {
		t.Error("a change of replicas must wake the console")
	}
	ck.t = ck.t.Add(time.Minute)
	feed(c, ck, c.targets[0], []Sample{recv("s", 3)})
	s := c.Snapshot(Gateway, 5, nil)
	if o := s.Owners[0]; o.Replicas != 1 || o.Reachable != 1 || o.State == "partial" {
		t.Fatalf("a scaled-down replica must not stay a silent one: %+v", o)
	}
	// Its past minute is still in the sums.
	val(t, c, 4, 1)
	// A pod that is not announced and has no owner left is gone entirely.
	c.SetTargets(nil)
	if c.Configured(Gateway) || len(c.Snapshot(Gateway, 5, nil).Owners) != 0 {
		t.Fatal("no pod announced, so no source")
	}
}

func TestSetTargetsKeepsBaselineAndCaps(t *testing.T) {
	c, ck := newTest()
	one := []Target{{Kind: Gateway, Owner: "g", url: "http://10.0.0.1:9090/metrics"}}
	c.SetTargets(one)
	feed(c, ck, c.targets[0], []Sample{recv("s", 5)})
	c.SetTargets(one)
	if len(c.targets) != 1 || c.targets[0].prev == nil {
		t.Fatal("an unchanged pod must keep its baseline")
	}
	var many []Target
	for i := range maxTargets + 10 {
		many = append(many, Target{Kind: Gateway, Owner: "g", url: "http://10.0.1." + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ":9090/metrics"})
	}
	c.SetTargets(many)
	live := 0
	for _, tg := range c.targets {
		if tg.retired.IsZero() {
			live++
		}
	}
	if live != maxTargets {
		t.Fatalf("%d live targets, want the cap %d", live, maxTargets)
	}
}

// A pod that restarts in a loop is a new address every round: the retired ones
// are bounded, the oldest go first, and the live ones are never touched.
func TestRetiredTargetsAreBounded(t *testing.T) {
	c, ck := newTest()
	addr := func(i int) Target {
		return Target{Kind: Gateway, Owner: "g", url: fmt.Sprintf("http://10.1.%d.%d:9090/metrics", i/250, i%250)}
	}
	for i := range 3 * maxRetired {
		ck.t = ck.t.Add(time.Second)
		c.SetTargets([]Target{addr(i)}) // each round the previous address is retired
	}
	retired, live := 0, 0
	for _, tg := range c.targets {
		if tg.retired.IsZero() {
			live++
		} else {
			retired++
		}
	}
	if live != 1 || retired > maxRetired {
		t.Fatalf("%d live, %d retired; want 1 live and at most %d retired", live, retired, maxRetired)
	}
	if want := addr(3*maxRetired - 1).url; c.targets[len(c.targets)-1].url != want {
		t.Fatalf("the live target was not kept")
	}
}

type fakeSource struct {
	mu   sync.Mutex
	pods []k8s.Pod
	err  error
}

func (f *fakeSource) set(p []k8s.Pod, err error) { f.mu.Lock(); f.pods, f.err = p, err; f.mu.Unlock() }
func (f *fakeSource) ListPods(context.Context) ([]k8s.Pod, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]k8s.Pod(nil), f.pods...), f.err
}

func TestDiscoverFollowsThePodsAndSurvivesAPIErrors(t *testing.T) {
	src := &fakeSource{}
	src.set([]k8s.Pod{pod("a", "10.0.0.1", true, gw("g")), pod("b", "10.0.0.2", true, gw("g"))}, nil)
	c := New(Config{Interval: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Discover(ctx, src, c, 10*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
		close(done)
	}()
	count := func() int {
		n := 0
		c.mu.RLock()
		defer c.mu.RUnlock()
		for _, tg := range c.targets {
			if tg.retired.IsZero() {
				n++
			}
		}
		return n
	}
	waitFor := func(want int) {
		t.Helper()
		for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
			if count() == want {
				return
			}
		}
		t.Fatalf("%d live targets, want %d", count(), want)
	}
	waitFor(2)
	src.set(nil, errors.New("pods is forbidden"))
	time.Sleep(60 * time.Millisecond)
	if count() != 2 {
		t.Fatal("an API error must keep the previous targets")
	}
	src.set([]k8s.Pod{pod("a", "10.0.0.1", true, gw("g"))}, nil)
	waitFor(1)
	cancel()
	<-done
}

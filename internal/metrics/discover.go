package metrics

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	k8s "brevis.sh/brevis/internal/execution/kubernetes"
)

// The annotations a gateway or SQL API pod puts on its pod template to be
// found, the way Prometheus' kubernetes_sd reads prometheus.io/*.
const (
	AnnotationKind = "brevis.sh/metrics-kind" // gateway | endpoints
	AnnotationName = "brevis.sh/metrics-name" // the gateway's `name:` or the API's name
	AnnotationPort = "brevis.sh/metrics-port" // default 9090
	AnnotationPath = "brevis.sh/metrics-path" // default /metrics
)

// PodSource lists the Running pods of this process's own namespace.
type PodSource interface {
	ListPods(ctx context.Context) ([]k8s.Pod, error)
}

// TargetsFromPods turns the Running, Ready, annotated pods into targets: one
// per pod, which is one per replica. A pod that is not ready, is shutting
// down, has no address, or carries an annotation that does not parse is
// skipped; it is never an error, because someone else's pod must not be able
// to stop the console's metrics.
//
// The address is the pod IP from the API server's answer and nothing else, and
// it must be an ordinary unicast address: loopback, link-local (the cloud
// metadata service lives there) and unspecified ones are refused, so an
// annotation cannot aim this process at them.
func TargetsFromPods(pods []k8s.Pod) []Target {
	sort.Slice(pods, func(i, j int) bool { return pods[i].Metadata.Name < pods[j].Metadata.Name })
	var out []Target
	for _, p := range pods {
		if t, ok := targetOf(p); ok {
			out = append(out, t)
		}
	}
	return out
}

func targetOf(p k8s.Pod) (Target, bool) {
	a := p.Metadata.Annotations
	kind := Kind(a[AnnotationKind])
	if kind != Gateway && kind != Endpoints {
		return Target{}, false
	}
	owner := strings.TrimSpace(a[AnnotationName])
	if owner == "" || len(owner) > maxLabelValue || strings.ContainsFunc(owner, func(r rune) bool { return r < ' ' || r == 0x7f }) {
		return Target{}, false
	}
	st := p.Status
	if st == nil || st.Phase != "Running" || p.Metadata.DeletionTimestamp != "" || !ready(st) {
		return Target{}, false
	}
	// A pod on the node's network answers on the node's address: scraping it is
	// scraping the node. Refused by the spec and by the address, in case a
	// spec is not what the kubelet ran.
	if p.Spec.HostNetwork || (st.HostIP != "" && st.HostIP == st.PodIP) {
		return Target{}, false
	}
	ip, err := netip.ParseAddr(st.PodIP)
	if err != nil || !ip.IsValid() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return Target{}, false
	}
	port := 9090
	if v, set := a[AnnotationPort]; set {
		if port, err = strconv.Atoi(v); err != nil || port < 1 || port > 65535 {
			return Target{}, false
		}
	}
	// Only a port the pod declares: an annotation picks among the pod's own
	// listeners and cannot aim this process at another service of the pod's IP.
	if !declares(p.Spec, port) {
		return Target{}, false
	}
	path := "/metrics"
	if v, set := a[AnnotationPath]; set {
		path = v
		// A path, not a way to change the host or add a query.
		if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "?#@\\ \t\r\n") {
			return Target{}, false
		}
	}
	return Target{Kind: kind, Owner: owner, url: "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(port)) + path}, true
}

func declares(spec k8s.PodSpec, port int) bool {
	for _, c := range spec.Containers {
		for _, cp := range c.Ports {
			if cp.ContainerPort == port {
				return true
			}
		}
	}
	return false
}

func ready(st *k8s.PodStatus) bool {
	for _, c := range st.Conditions {
		if c.Type == "Ready" {
			return c.Status == "True"
		}
	}
	return false
}

// Discover keeps c's targets equal to what the pods announce, listing every
// `every` until ctx ends. A failed listing (RBAC, API server down) keeps the
// previous targets: the replicas did not vanish because the API server
// hiccuped, and each target already says when it stops answering. A pod that
// is gone from a successful listing drops out at once and does not stay
// behind as a silent replica.
func Discover(ctx context.Context, src PodSource, c *Collector, every time.Duration, log *slog.Logger) {
	failing := false
	for {
		pods, err := src.ListPods(ctx)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			if !failing { // once per streak, not once per tick
				log.Warn("listing pods for metrics discovery failed; keeping the last known sources", "error", err)
			}
			failing = true
		default:
			failing = false
			ts := TargetsFromPods(pods)
			if len(ts) > maxTargets {
				log.Warn("more annotated pods than the console reads; the rest are ignored", "found", len(ts), "max", maxTargets)
			}
			c.SetTargets(ts)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}

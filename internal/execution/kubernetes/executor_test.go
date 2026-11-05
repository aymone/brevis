package kubernetes_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"brevis.sh/brevis/internal/execution"
	k8s "brevis.sh/brevis/internal/execution/kubernetes"
)

// fakeAPI stands in for the API server: a sequence of phases, the log and a
// record of what was asked. Testing the whole cycle with no cluster is what makes
// this
// path verifiable in CI.
type fakeAPI struct {
	mu sync.Mutex

	phases    []k8s.Pod // returned in order, the last one repeats
	read      int
	log       string
	logErr    error
	createErr error

	created []k8s.Pod
	deleted []string
}

func (a *fakeAPI) CreatePod(_ context.Context, p k8s.Pod) (k8s.Pod, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.createErr != nil {
		return k8s.Pod{}, a.createErr
	}
	a.created = append(a.created, p)
	return p, nil
}

func (a *fakeAPI) GetPod(_ context.Context, _ string) (k8s.Pod, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	i := a.read
	if i >= len(a.phases) {
		i = len(a.phases) - 1
	}
	a.read++
	return a.phases[i], nil
}

func (a *fakeAPI) Logs(_ context.Context, _ string, _ bool) (io.ReadCloser, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.logErr != nil {
		return nil, a.logErr
	}
	return io.NopCloser(strings.NewReader(a.log)), nil
}

func (a *fakeAPI) DeletePod(_ context.Context, name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.deleted = append(a.deleted, name)
	return nil
}

func phase(f string) k8s.Pod {
	return k8s.Pod{
		Metadata: k8s.Metadata{Name: "p"},
		Status:   &k8s.PodStatus{Phase: f},
	}
}

func withOutput(f string, code int) k8s.Pod {
	p := phase(f)
	var st k8s.StatusContainer
	st.Name = "step"
	st.State.Terminated = &struct {
		ExitCode int    `json:"exitCode"`
		Reason   string `json:"reason"`
		Message  string `json:"message"`
	}{ExitCode: code}
	p.Status.ContainerStatuses = []k8s.StatusContainer{st}
	return p
}

func runStep(t *testing.T, api *fakeAPI, tk execution.TaskExec) []execution.Event {
	t.Helper()
	e := k8s.NewExecutor(api, k8s.Options{Namespace: "data"})
	e.Interval = time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := e.Execute(ctx, tk)
	if err != nil {
		t.Fatal(err)
	}
	var out []execution.Event
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func TestASuccessfulPodReportsTheLogAndIsDeleted(t *testing.T) {
	api := &fakeAPI{
		phases: []k8s.Pod{phase("Pending"), phase("Running"), withOutput("Succeeded", 0)},
		log:    "Running with dbt=1.10.3\nCompleted successfully\n",
	}
	events := runStep(t, api, task())

	var succeeded bool
	var lines []string
	for _, e := range events {
		switch e.Kind {
		case execution.EventSucceeded:
			succeeded = true
		case execution.EventLog:
			lines = append(lines, e.Message)
		}
	}
	if !succeeded {
		t.Fatalf("no success event: %+v", events)
	}
	if len(lines) == 0 || !strings.Contains(strings.Join(lines, "\n"), "Completed successfully") {
		t.Errorf("the pod's log did not arrive: %v", lines)
	}
	if len(api.deleted) != 1 {
		t.Errorf("a successful pod has to be deleted (deleted=%v)", api.deleted)
	}
	if len(api.created) != 1 || api.created[0].Spec.Containers[0].Image != task().Image {
		t.Errorf("pod created wrong: %+v", api.created)
	}
}

func TestAFailingPodCarriesTheExitCode(t *testing.T) {
	api := &fakeAPI{
		phases: []k8s.Pod{phase("Running"), withOutput("Failed", 2)},
		log:    "Database Error in model x\n",
	}
	events := runStep(t, api, task())

	var failure *execution.Event
	for i := range events {
		if events[i].Kind == execution.EventFailed {
			failure = &events[i]
		}
	}
	if failure == nil {
		t.Fatalf("no failure event: %+v", events)
	}
	if failure.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2", failure.ExitCode)
	}
	if !strings.Contains(failure.Message, "code 2") {
		t.Errorf("message = %q", failure.Message)
	}
}

// Kubernetes's reason tells "the code failed" from "the cluster killed the
// process" -- OOMKilled and DeadlineExceeded call for opposite actions.
func TestTheClustersReasonShowsInTheFailure(t *testing.T) {
	dead := withOutput("Failed", 137)
	dead.Status.Reason = "DeadlineExceeded"
	api := &fakeAPI{phases: []k8s.Pod{phase("Running"), dead}}

	for _, e := range runStep(t, api, task()) {
		if e.Kind == execution.EventFailed {
			if !strings.Contains(e.Message, "DeadlineExceeded") {
				t.Errorf("the message has no reason from the cluster: %q", e.Message)
			}
			return
		}
	}
	t.Fatal("no failure event")
}

// A pod stuck in ImagePullBackOff produces no log at all: without reporting the
// reason, the step would look hung until the timeout, with no line explaining.
func TestTheReasonForWaitingIsReported(t *testing.T) {
	stuck := phase("Pending")
	var st k8s.StatusContainer
	st.Name = "step"
	st.State.Waiting = &struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}{Reason: "ImagePullBackOff", Message: "Back-off pulling image"}
	stuck.Status.ContainerStatuses = []k8s.StatusContainer{st}

	api := &fakeAPI{phases: []k8s.Pod{stuck, stuck, withOutput("Failed", 1)}}

	var found bool
	for _, e := range runStep(t, api, task()) {
		if e.Kind == execution.EventLog && strings.Contains(e.Message, "ImagePullBackOff") {
			found = true
		}
	}
	if !found {
		t.Error("the reason for waiting was not reported -- the step would look hung with no explanation")
	}
}

// A deterministic name: a pod that already exists was created by a run that
// died before recording it. Adopting it avoids starting a second one running
// the same dbt.
func TestAnAlreadyExistingPodIsAdopted(t *testing.T) {
	api := &fakeAPI{
		createErr: errors.New(`pods "x" already exists`),
		phases:    []k8s.Pod{withOutput("Succeeded", 0)},
	}
	events := runStep(t, api, task())
	for _, e := range events {
		if e.Kind == execution.EventSucceeded {
			return
		}
	}
	t.Fatalf("an existing pod should be followed, not refused: %+v", events)
}

// A creation failure (RBAC, quota, an invalid image) has to surface as the step's
// error, rather than become a ghost pod nobody follows.
func TestACreationErrorReachesTheCaller(t *testing.T) {
	api := &fakeAPI{createErr: errors.New(`pods is forbidden: cannot create resource "pods"`)}
	e := k8s.NewExecutor(api, k8s.Options{})
	if _, err := e.Execute(context.Background(), task()); err == nil {
		t.Fatal("expected an error")
	} else if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("the error lost the cause: %v", err)
	}
}

func TestAFailedPodMayBeKeptForInspection(t *testing.T) {
	api := &fakeAPI{phases: []k8s.Pod{withOutput("Failed", 1)}}
	e := k8s.NewExecutor(api, k8s.Options{KeepFailedPod: true})
	e.Interval = time.Millisecond

	ch, err := e.Execute(context.Background(), task())
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
	if len(api.deleted) != 0 {
		t.Errorf("a failed pod was deleted despite the option: %v", api.deleted)
	}
}

// `Pending` is not an error to Kubernetes: a pod that fits on no node stays there
// forever. Without a cutoff the stage waits along with it -- no failure and no
// retry -- which is how a CPU request larger than the pool's free capacity hung a
// whole run in dev.
func TestAPodThatDoesNotStartFailsWithTheSchedulersReason(t *testing.T) {
	stuck := phase("Pending")
	stuck.Status.Conditions = []k8s.Condition{{
		Type: "PodScheduled", Status: "False", Reason: "Unschedulable",
		Message: "0/8 nodes are available: 6 Insufficient cpu.",
	}}
	api := &fakeAPI{phases: []k8s.Pod{stuck}}

	e := k8s.NewExecutor(api, k8s.Options{StartTimeout: 30 * time.Millisecond})
	e.Interval = time.Millisecond

	ch, err := e.Execute(context.Background(), task())
	if err != nil {
		t.Fatal(err)
	}
	var failure *execution.Event
	for ev := range ch {
		if ev.Kind == execution.EventFailed {
			e := ev
			failure = &e
		}
	}
	if failure == nil {
		t.Fatal("the step never failed -- it would stay stuck forever")
	}
	if !strings.Contains(failure.Message, "Insufficient cpu") {
		t.Errorf("the message does not say why it was not scheduled: %q", failure.Message)
	}
}

// The pod's name distinguishes THE RUN's attempt: without that the dispatcher's
// retry (which restarts the run from scratch) finds the previous attempt's pod
// again.
func TestTheNameChangesWithTheRunsAttempt(t *testing.T) {
	a := task()
	b := task()
	b.RunAttempt = 1
	if k8s.PodName(a) == k8s.PodName(b) {
		t.Error("different attempts of the run produced the same pod")
	}
}

// KeepFailedPod keeps a pod that FINISHED failing, for somebody to read. A pod
// whose run was canceled has not finished -- it is still running its step --
// and keeping it keeps that step running beside the run's next attempt, which
// is the duplicate execution #121 exists to end. It is deleted.
func TestACanceledPodIsDeletedEvenWhenFailedOnesAreKept(t *testing.T) {
	api := &fakeAPI{phases: []k8s.Pod{phase("Running")}}
	e := k8s.NewExecutor(api, k8s.Options{KeepFailedPod: true})
	e.Interval = time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := e.Execute(ctx, task())
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	cancel()
	for range ch {
	}

	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.deleted) != 1 {
		t.Errorf("a canceled pod, still running, was kept (deleted=%v)", api.deleted)
	}
}

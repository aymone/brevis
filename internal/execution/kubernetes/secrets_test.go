package kubernetes

import (
	"encoding/json"
	"strings"
	"testing"

	"brevis.sh/brevis/internal/execution"
)

// optionsAllowing is what an installation that authorized this Secret passes.
func optionsAllowing() Options {
	return Options{AllowedSecrets: []string{"gabriel-session", "vault"}}.withDefaults()
}

func taskWithSecret() execution.TaskExec {
	return execution.TaskExec{
		NodeID:  "fetch_occurrences",
		Image:   "data-pipeline-go:local",
		Command: "/usr/local/bin/gabriel",
		Env:     map[string]string{"BREVIS_LOG_LEVEL": "info"},
		Secrets: map[string]string{"GABRIEL_SESSION_COOKIE": "gabriel-session/cookie"},
	}
}

// TestASecretBecomesASecretKeyRef: the pod references the key, the kubelet
// resolves it. The engine never sees the value.
func TestASecretBecomesASecretKeyRef(t *testing.T) {
	pod, err := BuildPod(taskWithSecret(), optionsAllowing())
	if err != nil {
		t.Fatalf("BuildPod: %v", err)
	}

	var found *Var
	for i, v := range pod.Spec.Containers[0].Env {
		if v.Name == "GABRIEL_SESSION_COOKIE" {
			found = &pod.Spec.Containers[0].Env[i]
		}
	}
	if found == nil {
		t.Fatal("the secret's variable did not go into the container")
	}
	if found.Value != "" {
		t.Errorf("the value was materialized into the pod: %q", found.Value)
	}
	if found.ValueFrom == nil || found.ValueFrom.SecretKeyRef == nil {
		t.Fatal("no valueFrom.secretKeyRef")
	}
	if got := found.ValueFrom.SecretKeyRef; got.Name != "gabriel-session" || got.Key != "cookie" {
		t.Errorf("wrong coordinate: %+v", got)
	}
}

// TestThePodsJSONSendsNoEmptyValueAlongsideValueFrom: an EMPTY value next to
// valueFrom: the server refuses the two keys on the same variable, and the refusal talks about a container field,
// not about a line of YAML.
func TestThePodsJSONSendsNoEmptyValueAlongsideValueFrom(t *testing.T) {
	pod, err := BuildPod(taskWithSecret(), optionsAllowing())
	if err != nil {
		t.Fatalf("BuildPod: %v", err)
	}
	b, err := json.Marshal(pod)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), `"value":""`) {
		t.Errorf("the JSON sends an empty value next to valueFrom:\n%s", b)
	}
}

// TestTheSecretDoesNotLeakInThePodsJSON: the manifest goes to the server, shows
// up in
// `kubectl get pod -o yaml` and tends to end up in a deploy log.
func TestTheSecretDoesNotLeakInThePodsJSON(t *testing.T) {
	task := taskWithSecret()
	task.Secrets["GABRIEL_SESSION_COOKIE"] = "gabriel-session/cookie"

	pod, err := BuildPod(task, optionsAllowing())
	if err != nil {
		t.Fatalf("BuildPod: %v", err)
	}
	b, _ := json.Marshal(pod)
	// The coordinate may appear; what may not is a secret's value, and the engine
	// has none to leak -- this test pins that down.
	if strings.Contains(string(b), "gabriel-session/cookie") {
		t.Errorf("the raw coordinate went into the manifest instead of the secretKeyRef:\n%s", b)
	}
}

// TestThePodsEnvironmentIsDeterministic: two identical pods have to produce the
// same JSON, otherwise the diff between two deploys becomes map-ordering noise.
func TestThePodsEnvironmentIsDeterministic(t *testing.T) {
	task := taskWithSecret()
	task.Env["A"] = "1"
	task.Env["Z"] = "2"
	task.Secrets["OTHER"] = "vault/key"

	var first string
	for i := 0; i < 20; i++ {
		pod, err := BuildPod(task, optionsAllowing())
		if err != nil {
			t.Fatalf("BuildPod: %v", err)
		}
		b, _ := json.Marshal(pod.Spec.Containers[0].Env)
		if i == 0 {
			first = string(b)
			continue
		}
		if string(b) != first {
			t.Fatalf("unstable order:\n%s\n%s", first, b)
		}
	}
}

// TestTheYAMLDoesNotChooseWhichSecretToMount: `secrets:` inverts who chooses --
// EnvFromSecrets comes from the scheduler's environment, `secrets:` comes from
// the file, and the file is written by somebody else. Without the list, a
// workflow could mount
// any Secret in the namespace, Brevis's own database one included,
// and would run an arbitrary command with it in hand.
func TestTheYAMLDoesNotChooseWhichSecretToMount(t *testing.T) {
	task := taskWithSecret()
	task.Secrets["STOLEN"] = "brevis-database/url"

	_, err := BuildPod(task, optionsAllowing())
	if err == nil {
		t.Fatal("the YAML mounted a Secret the installation did not allow")
	}
	for _, required := range []string{"brevis-database", "BREVIS_POD_ALLOWED_SECRETS"} {
		if !strings.Contains(err.Error(), required) {
			t.Errorf("the error does not say %q: %v", required, err)
		}
	}
}

// TestWithNoListNoSecretGetsThrough: denying by default costs one variable in the
// installation; allowing by default costs the reverse, and the reverse cannot be
// undone.
func TestWithNoListNoSecretGetsThrough(t *testing.T) {
	_, err := BuildPod(taskWithSecret(), Options{}.withDefaults())
	if err == nil {
		t.Fatal("with no allow-list, the Secret got through")
	}
	if !strings.Contains(err.Error(), "neither is any") {
		t.Errorf("the error does not explain that the list is empty: %v", err)
	}
}

// TestWithNoSecretsInTheYAMLNothingChanges: a workflow that does not use
// `secrets:` must not
// start requiring new configuration.
func TestWithNoSecretsInTheYAMLNothingChanges(t *testing.T) {
	task := taskWithSecret()
	task.Secrets = nil

	if _, err := BuildPod(task, Options{}.withDefaults()); err != nil {
		t.Errorf("a workflow with no secrets started failing: %v", err)
	}
}

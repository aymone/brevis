package local

import (
	"os"
	"strings"
	"testing"

	"brevis.sh/brevis/internal/execution"
)

// TestALocalSecretComesFromTheEnginesEnvironment: in Kubernetes the coordinate points at a
// Secret; in local there is no Secret at all, so the engine reads the variable of
// the same
// name from its own environment.
func TestALocalSecretComesFromTheEnginesEnvironment(t *testing.T) {
	t.Setenv("GABRIEL_SESSION_COOKIE", "session=abc==")

	env, err := taskEnvironment(execution.TaskExec{
		NodeID:  "fetch_occurrences",
		Env:     map[string]string{"BREVIS_LOG_LEVEL": "info"},
		Secrets: map[string]string{"GABRIEL_SESSION_COOKIE": "gabriel-session/cookie"},
	})
	if err != nil {
		t.Fatalf("taskEnvironment: %v", err)
	}
	if !contains(env, "GABRIEL_SESSION_COOKIE=session=abc==") {
		t.Errorf("the secret did not reach the process: %v", env)
	}
	if !contains(env, "BREVIS_LOG_LEVEL=info") {
		t.Errorf("the literal env vanished: %v", env)
	}
}

// TestAMissingSecretFailsBeforeRunning: an empty string would become an empty cookie and
// a 401 further down, blaming the API for a variable nobody exported.
func TestAMissingSecretFailsBeforeRunning(t *testing.T) {
	cases := map[string]func(*testing.T){
		// Setenv registers the cleanup; Unsetenv right after leaves the variable
		// genuinely absent and the original value comes back at the end of the
		// test.
		"not set": func(t *testing.T) {
			t.Setenv("GABRIEL_SESSION_COOKIE", "x")
			if err := os.Unsetenv("GABRIEL_SESSION_COOKIE"); err != nil {
				t.Fatal(err)
			}
		},
		"empty": func(t *testing.T) { t.Setenv("GABRIEL_SESSION_COOKIE", "") },
	}
	for name, prepare := range cases {
		t.Run(name, func(t *testing.T) {
			prepare(t)

			_, err := taskEnvironment(execution.TaskExec{
				NodeID:  "fetch_occurrences",
				Secrets: map[string]string{"GABRIEL_SESSION_COOKIE": "gabriel-session/cookie"},
			})
			if err == nil {
				t.Fatal("a missing secret got through")
			}
			for _, required := range []string{"GABRIEL_SESSION_COOKIE", "fetch_occurrences", "gabriel-session/cookie"} {
				if !strings.Contains(err.Error(), required) {
					t.Errorf("the error does not say %q: %v", required, err)
				}
			}
		})
	}
}

// TestATaskDoesNotInheritTheEnginesEnvironmentByAccident: the rule that already
// existed. `secrets:` is the by-name opt-in against it, not an open gate.
func TestATaskDoesNotInheritTheEnginesEnvironmentByAccident(t *testing.T) {
	t.Setenv("ORCHESTRATOR_SECRET", "should not leak")

	env, err := taskEnvironment(execution.TaskExec{NodeID: "a", Env: map[string]string{"OK": "1"}})
	if err != nil {
		t.Fatalf("taskEnvironment: %v", err)
	}
	for _, kv := range env {
		if strings.HasPrefix(kv, "ORCHESTRATOR_SECRET=") {
			t.Errorf("the task inherited the engine's environment: %v", env)
		}
	}
}

// TestASecretOverridesTheLiteral: if both exist on the same task, the secret
// wins -- but the domain already refuses the collision, so this only pins down
// that the order is not accidental.
func TestASecretOverridesTheLiteral(t *testing.T) {
	t.Setenv("TOKEN", "from-environment")

	env, err := taskEnvironment(execution.TaskExec{
		NodeID:  "a",
		Env:     map[string]string{"TOKEN": "literal"},
		Secrets: map[string]string{"TOKEN": "vault/token"},
	})
	if err != nil {
		t.Fatalf("taskEnvironment: %v", err)
	}
	if !contains(env, "TOKEN=from-environment") {
		t.Errorf("the literal beat the secret: %v", env)
	}
}

func contains(env []string, wanted string) bool {
	for _, kv := range env {
		if kv == wanted {
			return true
		}
	}
	return false
}

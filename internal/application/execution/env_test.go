package execution

import (
	run "brevis.sh/brevis/internal/domain/run"
	"testing"

	wf "brevis.sh/brevis/internal/domain/workflow"
)

// TestTheEnvironmentsPrecedence: weakest to strongest -- the engine's global
// environment, the workflow's `env:`, the step's `env:`.
//
// The step beating the global is the part that changed. The other way round, a
// variable declared in the file would lose quietly to a BREVIS_TASK_ENV somebody
// configured months ago -- and "loses quietly" is the failure mode this
// project chases hardest.
func TestTheEnvironmentsPrecedence(t *testing.T) {
	w := wf.Workflow{
		Slug: "x",
		Env:  map[string]string{"NIVEL": "workflow", "SO_DO_WORKFLOW": "1"},
		Nodes: []wf.Node{
			{ID: "com_env", Run: "echo", Env: map[string]string{"NIVEL": "passo"}},
			{ID: "sem_env", Run: "echo"},
		},
	}
	r := Runner{Env: map[string]string{"NIVEL": "global", "SO_DO_GLOBAL": "1"}}

	cases := map[string]string{"com_env": "passo", "sem_env": "workflow"}
	for _, n := range w.Nodes {
		env := mergeEnv(r.Env, r.runContext(instance{key: run.Step(n.ID)}, false, 0), w.EnvOf(n))

		if got := env["NIVEL"]; got != cases[n.ID] {
			t.Errorf("%s: LEVEL = %q, expected %q", n.ID, got, cases[n.ID])
		}
		// Inheriting must not mean losing the rest.
		if env["SO_DO_WORKFLOW"] != "1" {
			t.Errorf("%s: it lost the variable only the workflow declared", n.ID)
		}
		if env["SO_DO_GLOBAL"] != "1" {
			t.Errorf("%s: it lost the variable only the global declared", n.ID)
		}
	}
}

// TestSecretsDoNotEnterTheTasksEnv: if they did, the value would have to be
// resolved at assembly time -- and would pass through the dispatcher, through
// the log and through any TaskExec dump somebody writes later.
func TestSecretsDoNotEnterTheTasksEnv(t *testing.T) {
	w := wf.Workflow{
		Slug:    "x",
		Secrets: map[string]string{"TOKEN": "cofre/token"},
		Nodes:   []wf.Node{{ID: "a", Run: "echo"}},
	}
	n := w.Nodes[0]

	env := mergeEnv(nil, nil, w.EnvOf(n))
	if _, has := env["TOKEN"]; has {
		t.Error("the secret went into the task's Env")
	}
	if w.SecretsOf(n)["TOKEN"] != "cofre/token" {
		t.Error("the secret did not arrive as a coordinate")
	}
}

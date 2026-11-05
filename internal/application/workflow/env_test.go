package workflow

import (
	"strings"
	"testing"
)

const gabrielsFile = `
name: vendors_gabriel_occurrence
type: chain
env:
  BREVIS_LOG_LEVEL: info
steps:
  - id: fetch_occurrences
    image: data-pipeline-go:local
    shell: false
    run: /usr/local/bin/gabriel
    secrets:
      GABRIEL_SESSION_COOKIE: gabriel-session/cookie
    env:
      BREVIS_LOG_LEVEL: debug
  - id: dbt_build
    image: data-pipeline-dbt:local
    run: dbt build
`

// TestEnvIsInheritedFromTheWorkflowAndTheStepOverrides: the same rule as `image` and
// `resources`, and name by name -- a step that changes the log level does not
// lose the
// workflow's other variables.
func TestEnvIsInheritedFromTheWorkflowAndTheStepOverrides(t *testing.T) {
	w, err := Parse("gabriel.yaml", []byte(gabrielsFile))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	byID := map[string]int{}
	for i, n := range w.Nodes {
		byID[n.ID] = i
	}

	fetch := w.Nodes[byID["fetch_occurrences"]]
	if got := w.EnvOf(fetch)["BREVIS_LOG_LEVEL"]; got != "debug" {
		t.Errorf("the step declared debug and ended up %q", got)
	}

	dbt := w.Nodes[byID["dbt_build"]]
	if got := w.EnvOf(dbt)["BREVIS_LOG_LEVEL"]; got != "info" {
		t.Errorf("the step with no env should inherit info, it ended up %q", got)
	}
}

// TestASecretOnlyGoesToTheStepThatDeclaredIt: it is the reason the per-step key
// exists. With BREVIS_POD_ENV_FROM_SECRETS the cookie also went into dbt's pod,
// which does not need it.
func TestASecretOnlyGoesToTheStepThatDeclaredIt(t *testing.T) {
	w, err := Parse("gabriel.yaml", []byte(gabrielsFile))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, n := range w.Nodes {
		_, has := w.SecretsOf(n)["GABRIEL_SESSION_COOKIE"]
		if n.ID == "fetch_occurrences" && !has {
			t.Error("the step that declared the secret did not get it")
		}
		if n.ID == "dbt_build" && has {
			t.Error("the cookie leaked into the dbt step, which did not declare it")
		}
	}
}

// TestSecretsRefusesWhatIsNotACoordinate: the value is where to find it, never
// the secret. A value that is not `secret/key` is almost always somebody pasting
// the real value into the file -- and the file is in git.
func TestSecretsRefusesWhatIsNotACoordinate(t *testing.T) {
	cases := map[string]string{
		"no slash":          "GABRIEL_SESSION_COOKIE: eyJhbGciOiJkaXIi==",
		"empty secret":      "GABRIEL_SESSION_COOKIE: /cookie",
		"empty key":         "GABRIEL_SESSION_COOKIE: gabriel-session/",
		"too many slashes":  "GABRIEL_SESSION_COOKIE: ns/gabriel-session/cookie",
		"invalid name":      "GABRIEL-SESSION-COOKIE: gabriel-session/cookie",
		"name with a space": "'GABRIEL COOKIE': gabriel-session/cookie",
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			yaml := "name: x\ntype: chain\nsteps:\n  - id: a\n    run: echo\n    secrets:\n      " + line + "\n"
			if _, err := Parse("x.yaml", []byte(yaml)); err == nil {
				t.Fatalf("it accepted %q", line)
			}
		})
	}
}

// TestEnvAndSecretsCannotCollide: the same variable with a literal value and
// coming from a secret is ambiguous, and any tie-break would be a rule nobody
// remembers.
func TestEnvAndSecretsCannotCollide(t *testing.T) {
	yaml := `
name: x
type: chain
env:
  TOKEN: literal
steps:
  - id: a
    run: echo
    secrets:
      TOKEN: vault/token
`
	_, err := Parse("x.yaml", []byte(yaml))
	if err == nil {
		t.Fatal("accepted the same variable in env and in secrets")
	}
	if !strings.Contains(err.Error(), "TOKEN") {
		t.Errorf("the error does not name the variable: %v", err)
	}
}

// TestASpaceInTheVariablesNameDoesNotGetThrough: `TOKEN : x` with a space before
// the
// dots and is valid YAML, and the space would travel with the name -- the pod
// starts, the binary
// does not find the variable, and nothing along the way says why.
func TestASpaceInTheVariablesNameDoesNotGetThrough(t *testing.T) {
	yaml := "name: x\ntype: chain\nsteps:\n  - id: a\n    run: echo\n    env:\n      ' TOKEN ': value\n"
	w, err := Parse("x.yaml", []byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, ok := w.EnvOf(w.Nodes[0])["TOKEN"]; !ok {
		t.Errorf("the name was not trimmed: %v", w.EnvOf(w.Nodes[0]))
	}
}

// TestACoordinateErrorDoesNotEchoTheSecret: the likeliest invalid coordinate is
// somebody having pasted the real secret -- and `brevis validate` runs in CI,
// whose log plenty of people read. That is what the first version did.
func TestACoordinateErrorDoesNotEchoTheSecret(t *testing.T) {
	const pasted = "eyJhbGciOiJkaXIiLCJlbmMiOiJBMjU2R0NNIn0..QUJDRA=="

	yaml := "name: x\ntype: chain\nsteps:\n  - id: a\n    run: echo\n    secrets:\n      TOKEN: " + pasted + "\n"
	_, err := Parse("x.yaml", []byte(yaml))
	if err == nil {
		t.Fatal("it accepted a pasted secret as a coordinate")
	}
	if strings.Contains(err.Error(), pasted) {
		t.Errorf("the error printed the secret:\n%v", err)
	}
	// And it still teaches the format, which is why the error exists.
	if !strings.Contains(err.Error(), "secret-name/key") {
		t.Errorf("the error stopped teaching the format: %v", err)
	}
}

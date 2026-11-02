package iceberg

import (
	"os"
	"strings"
	"testing"
)

// THE MODULE'S BOUNDARY IS THE DECISION, so the go.mod is tested like code.
//
// Three things must stay true for "you only pay for what you import" to hold
// at the module level, and each has a way of quietly going wrong: the module
// path, which a copy-paste from sdk/metrics/otelmeter would leave pointing
// at the wrong directory; the Go directive, which `go mod tidy` on a newer
// toolchain bumps on its own and which has to stay what the other modules
// declare; and a `replace`, which a developer adds to build against ../sdk
// and which hides, from every gate, what a consumer actually compiles against
// -- the exact failure gateway-consumer-check.sh exists to catch.
func TestTheModuleIsItsOwnAndPinsNothingLocally(t *testing.T) {
	raw, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	gomod := string(raw)

	if !strings.Contains(gomod, "\nmodule github.com/AreteAcademy/brevis/sdk/iceberg\n") {
		t.Error("the module path is not .../sdk/iceberg")
	}
	if !strings.Contains(gomod, "\ngo 1.26.0\n") {
		t.Error("the go directive is not 1.26.0, which is what sdk, gateway and sql declare")
	}
	for _, line := range strings.Split(gomod, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "replace ") {
			t.Errorf("a replace directive: %q -- this module requires the PUBLISHED sdk, like otelmeter, so a consumer and this module compile against the same thing", line)
		}
	}
}

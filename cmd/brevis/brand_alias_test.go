package main

import "testing"

// `marca` was the name of `brand` in a released version. A script that calls it
// must keep working, so the Portuguese name has to resolve to the same command.
func TestTheMarcaAliasStillReachesBrand(t *testing.T) {
	cmd, _, err := root().Find([]string{"marca"})
	if err != nil {
		t.Fatalf("`brevis marca` no longer resolves: %v", err)
	}
	if cmd.Name() != "brand" {
		t.Errorf("`brevis marca` resolves to %q, want brand", cmd.Name())
	}
}

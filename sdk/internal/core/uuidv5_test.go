package core

import (
	"math/rand"
	"testing"

	"github.com/google/uuid"
)

// TestUUIDv5AgreesWithThePackage is the net that allows reimplementing the
// formula
// frozen.
//
// A one-bit divergence here would change every ingestion_id already written -- a
// previous load would stop matching a new one, and nobody would notice until the
// duplicates showed up. So the claim is not "mine is right": it is "mine
// is identical to the uuid package's", over inputs nobody hand-picked.
func TestUUIDv5AgreesWithThePackage(t *testing.T) {
	r := rand.New(rand.NewSource(1))

	for i := 0; i < 5000; i++ {
		data := make([]byte, r.Intn(400))
		for j := range data {
			data[j] = byte(r.Intn(256))
		}

		var namespace uuid.UUID
		for j := range namespace {
			namespace[j] = byte(r.Intn(256))
		}

		want := uuid.NewSHA1(namespace, data)
		got := uuidV5(namespace, data)
		if got != want {
			t.Fatalf("diverged at %d bytes of data:\n  mine %s\n  uuid %s", len(data), got, want)
		}
	}
}

// TestUUIDv5InTheRealNamespace covers the case production uses, including the
// an empty key and one much larger than the stack buffer.
func TestUUIDv5InTheRealNamespace(t *testing.T) {
	entries := [][]byte{
		nil,
		[]byte(""),
		[]byte("open_meteo|hourly|123|2026-09-05T12:00:00Z"),
		make([]byte, 1000),
	}
	for _, data := range entries {
		if got, want := uuidV5(DefaultNamespace, data), uuid.NewSHA1(DefaultNamespace, data); got != want {
			t.Errorf("%d bytes: mine %s, uuid %s", len(data), got, want)
		}
	}
}

// TestFormatUUIDAgreesWithString: the canonical format is a contract too --
// it goes into the column.
func TestFormatUUIDAgreesWithString(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for i := 0; i < 2000; i++ {
		var u uuid.UUID
		for j := range u {
			u[j] = byte(r.Intn(256))
		}
		if got, want := formatUUID(u), u.String(); got != want {
			t.Fatalf("mine %q, String() %q", got, want)
		}
	}
}

// TestAKeyLargerThanTheStackBuffer: a 192-byte key fits on the stack, and a
// larger one falls to the heap -- both have to produce the same id.
func TestAKeyLargerThanTheStackBuffer(t *testing.T) {
	long := ""
	for len(long) < 300 {
		long += "provider-with-a-long-name-"
	}

	got, err := ComputeIngestionID(long, "entity", "key", "2026-09-05T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	want := uuid.NewSHA1(DefaultNamespace,
		[]byte(long+"|entity|key|2026-09-05T12:00:00Z")).String()
	if got != want {
		t.Errorf("long key diverged:\n  mine %s\n  uuid %s", got, want)
	}
}

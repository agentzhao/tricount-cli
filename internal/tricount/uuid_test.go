package tricount

import "testing"

func TestUUID5KnownVector(t *testing.T) {
	// uuid.uuid5(NAMESPACE_DNS, "python.org")
	got, err := uuid5("6ba7b810-9dad-11d1-80b4-00c04fd430c8", "python.org")
	if err != nil {
		t.Fatal(err)
	}
	if got != "886313e1-3b8a-5372-9b90-0c9aee199e5d" {
		t.Fatalf("uuid5 = %s", got)
	}
}

func TestIdempotencyUUIDStable(t *testing.T) {
	a, err := IdempotencyUUID("tABC", "fun-money:2026-10")
	if err != nil {
		t.Fatal(err)
	}
	b, err := IdempotencyUUID("tABC", "fun-money:2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("uuid changed: %s vs %s", a, b)
	}
	other, err := IdempotencyUUID("tABC", "fun-money:2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if other == a {
		t.Fatal("different keys produced the same uuid")
	}
	otherGroup, err := IdempotencyUUID("tXYZ", "fun-money:2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if otherGroup == a {
		t.Fatal("different groups produced the same uuid")
	}
	if _, err := IdempotencyUUID("", "key"); err == nil {
		t.Fatal("expected empty token to fail")
	}
}

package tricount

import "testing"

func TestFormatAndParseMinor(t *testing.T) {
	if got := FormatMinor(1250); got != "12.50" {
		t.Fatalf("FormatMinor(1250) = %s", got)
	}
	if got := FormatMinor(-50700); got != "-507.00" {
		t.Fatalf("FormatMinor(-50700) = %s", got)
	}
	minor, err := ParseAmountMinor("-507")
	if err != nil {
		t.Fatal(err)
	}
	if minor != -50700 {
		t.Fatalf("ParseAmountMinor(-507) = %d, want -50700 (507.00 major units, not 5.07)", minor)
	}
	if got := FormatMajor(50700); got != "507" {
		t.Fatalf("FormatMajor = %s", got)
	}
}

func TestSplitMinor(t *testing.T) {
	parts, err := SplitMinor(1000, 3)
	if err != nil {
		t.Fatal(err)
	}
	if parts[0] != 334 || parts[1] != 333 || parts[2] != 333 {
		t.Fatalf("split = %v", parts)
	}
}

func TestSplitRatio(t *testing.T) {
	parts, err := SplitRatio(1000, []int{1, 2, 1})
	if err != nil {
		t.Fatal(err)
	}
	if parts[0] != 250 || parts[1] != 500 || parts[2] != 250 {
		t.Fatalf("ratio = %v", parts)
	}
}

func TestNormalizeToken(t *testing.T) {
	cases := map[string]string{
		"tABC123xyz":                                 "tABC123xyz",
		"https://tricount.com/tABC123xyz":            "tABC123xyz",
		"https://tricount.com/tABC123xyz/":           "tABC123xyz",
		"https://www.tricount.com/en/tABC123xyz?x=1": "tABC123xyz",
		"tricount.com/tABC":                          "tABC",
	}
	for in, want := range cases {
		if got := NormalizeToken(in); got != want {
			t.Errorf("NormalizeToken(%q) = %q, want %q", in, got, want)
		}
	}
}

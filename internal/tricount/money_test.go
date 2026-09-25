package tricount

import "testing"

func TestParseMajorExact(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"12.50", 1250},
		{"12.5", 1250},
		{"12.500", 1250},
		{"1500", 150000},
		{"0.01", 1},
		{"0.10", 10},
		{".50", 50},
		{"8.70", 870},
		{"0.29", 29},
		{"1.15", 115},
		{"+12.50", 1250},
		{"90071992547409.91", 9007199254740991},
		{"92233720368547758.07", 9223372036854775807},
	}
	for _, tc := range cases {
		got, err := ParseMajorExact(tc.in)
		if err != nil {
			t.Fatalf("ParseMajorExact(%q) error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseMajorExact(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
	rejects := []string{"", "1.005", "0.005", "1.004", "12.501", "1e2", "1,000", "12.5.0", "abc", "-", "."}
	for _, in := range rejects {
		if _, err := ParseMajorExact(in); err == nil {
			t.Fatalf("ParseMajorExact(%q) succeeded, want error", in)
		}
	}
	if _, err := ParseMajorExact("92233720368547758.08"); err == nil {
		t.Fatal("expected overflow")
	}
}

func TestParseAmountMinorRoundsHalfAway(t *testing.T) {
	minor, err := ParseAmountMinor("1.005")
	if err != nil {
		t.Fatal(err)
	}
	if minor != 101 {
		t.Fatalf("ParseAmountMinor(1.005) = %d, want 101", minor)
	}
	minor, err = ParseAmountMinor("-1.005")
	if err != nil {
		t.Fatal(err)
	}
	if minor != -101 {
		t.Fatalf("ParseAmountMinor(-1.005) = %d, want -101", minor)
	}
	minor, err = ParseAmountMinor("1.004")
	if err != nil {
		t.Fatal(err)
	}
	if minor != 100 {
		t.Fatalf("ParseAmountMinor(1.004) = %d, want 100", minor)
	}
}

func TestParseRateAndConvert(t *testing.T) {
	got, err := ParseRate("150.50")
	if err != nil || got != "150.5" {
		t.Fatalf("ParseRate = %q %v", got, err)
	}
	if _, err := ParseRate("0"); err == nil {
		t.Fatal("expected zero rate to fail")
	}
	if _, err := ParseRate("-1.5"); err == nil {
		t.Fatal("expected negative rate to fail")
	}
	converted, err := ConvertMinor(1000, "1.5")
	if err != nil || converted != 1500 {
		t.Fatalf("ConvertMinor 1.5 = %d %v", converted, err)
	}
	converted, err = ConvertMinor(100, "1.005")
	if err != nil || converted != 101 {
		t.Fatalf("ConvertMinor half = %d %v, want 101", converted, err)
	}
	converted, err = ConvertMinor(1, "0.5")
	if err != nil || converted != 1 {
		t.Fatalf("ConvertMinor 0.5 = %d %v, want 1", converted, err)
	}
	converted, err = ConvertMinor(-1, "0.5")
	if err != nil || converted != -1 {
		t.Fatalf("ConvertMinor negative half = %d %v, want -1", converted, err)
	}
}

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

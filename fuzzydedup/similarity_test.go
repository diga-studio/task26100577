package fuzzydedup

import "testing"

func simOf(d, longer float64) float64 { return (1 - d/longer) * 100 }

func TestFieldSimilarity(t *testing.T) {
	cases := []struct {
		a, b string
		want float64
	}{
		{"", "", 100},
		{"abc", "abc", 100},
		{"abc", "abd", simOf(1, 3)},
		{"abcd", "ab", 50},
		{"北京中关村大街", "北京中关村大街", 100},
		{"北京中关村大街", "北京中关村小街", (1 - 1.0/7) * 100},
	}
	for _, c := range cases {
		if got := FieldSimilarity(c.a, c.b); got != c.want {
			diff := got - c.want
			if diff < 0 {
				diff = -diff
			}
			if diff > 1e-9 {
				t.Errorf("FieldSimilarity(%q,%q)=%v, want %v", c.a, c.b, got, c.want)
			}
		}
	}
}

func TestScorePairAverage(t *testing.T) {
	a := Record{ID: "1", Name: "abcd", Address: "wxyz"}
	b := Record{ID: "2", Name: "abcd", Address: "wx--"}
	s := ScorePair(a, b)
	if s.NameSimilarity != 100 {
		t.Errorf("NameSimilarity=%v, want 100", s.NameSimilarity)
	}
	if s.AddressSimilarity != 50 {
		t.Errorf("AddressSimilarity=%v, want 50", s.AddressSimilarity)
	}
	if s.Total != 75 {
		t.Errorf("Total=%v, want 75", s.Total)
	}
}

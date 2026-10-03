package fuzzydedup

import (
	"math/rand"
	"testing"
)

// refLevenshtein 是独立的全矩阵动态规划参考实现，用于交叉验证。
func refLevenshtein(a, b []rune) int {
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			d[i][j] = min3(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
		}
	}
	return d[len(a)][len(b)]
}

func TestLevenshteinKnown(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
		{"北京", "北京", 0},
		{"北京市", "北京", 1},
		{"上海浦东", "上海浦西", 1},
		{"株式会社アbc", "株式会社アbd", 1},
		{"🙂🙃🙂", "🙂🙂🙂", 1},
	}
	for _, c := range cases {
		if got := Levenshtein(c.a, c.b); got != c.want {
			t.Errorf("Levenshtein(%q,%q)=%d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLevenshteinFuzzVsReference(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	pool := []rune("ab中文字符🙂东京都01")
	for iter := 0; iter < 20000; iter++ {
		la := rng.Intn(70)
		lb := rng.Intn(70)
		a := make([]rune, la)
		b := make([]rune, lb)
		for i := range a {
			a[i] = pool[rng.Intn(len(pool))]
		}
		for i := range b {
			b[i] = pool[rng.Intn(len(pool))]
		}
		// 一半用例让 b 由 a 小幅变异而来，覆盖相似串。
		if iter%2 == 0 && la > 0 {
			b = append([]rune(nil), a...)
			for k := 0; k < rng.Intn(4); k++ {
				op := rng.Intn(3)
				pos := rng.Intn(len(b))
				switch op {
				case 0:
					b[pos] = pool[rng.Intn(len(pool))]
				case 1:
					b = append(b[:pos], b[pos+1:]...)
				case 2:
					b = append(b[:pos], append([]rune{pool[rng.Intn(len(pool))]}, b[pos:]...)...)
				}
				if len(b) == 0 {
					break
				}
			}
		}
		got := levenshteinRunes(a, b)
		want := refLevenshtein(a, b)
		if got != want {
			t.Fatalf("levenshteinRunes(%q,%q)=%d, want %d", string(a), string(b), got, want)
		}
	}
}

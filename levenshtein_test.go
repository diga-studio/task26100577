package dedup

import "testing"

func TestEditDistance(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "abc", 3},
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
		{"你好世界", "你好世", 1},
		{"你好世界", "你好世界", 0},
		{"北京市朝阳区", "北京市朝外区", 1},
		{"🙂🙃🙂", "🙂🙂🙂", 1},
		{"株式会社ABC", "株式会社ABD", 1},
	}
	for _, c := range cases {
		if got := EditDistance(c.a, c.b); got != c.want {
			t.Errorf("EditDistance(%q,%q)=%d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestBoundedEditDistance(t *testing.T) {
	cases := []struct {
		a, b    string
		maxDist int
		want    int
	}{
		{"kitten", "sitting", 3, 3},
		{"kitten", "sitting", 2, -1},
		{"abc", "abd", 0, -1},
		{"abc", "abc", 0, 0},
		{"", "", 0, 0},
		{"abcdef", "abc", 2, -1},
		{"abcdef", "abc", 3, 3},
		{"你好世界", "你好世", 1, 1},
		{"你好世界", "你好世", 0, -1},
	}
	for _, c := range cases {
		if got := boundedEditDistance([]rune(c.a), []rune(c.b), c.maxDist); got != c.want {
			t.Errorf("boundedEditDistance(%q,%q,%d)=%d, want %d", c.a, c.b, c.maxDist, got, c.want)
		}
	}
}

// TestBoundedMatchesFull 随机对拍：有界结果必须与完整距离一致。
func TestBoundedMatchesFull(t *testing.T) {
	rng := []rune("abcdefg你好🙂")
	seed := uint64(12345)
	rand := func(n int) int { // 简单 LCG，避免引入依赖
		seed = seed*6364136223846793005 + 1442695040888963407
		return int((seed >> 33) % uint64(n))
	}
	for iter := 0; iter < 20000; iter++ {
		la, lb := rand(12), rand(12)
		a := make([]rune, la)
		b := make([]rune, lb)
		for i := range a {
			a[i] = rng[rand(len(rng))]
		}
		for i := range b {
			b[i] = rng[rand(len(rng))]
		}
		full := editDistance(a, b)
		for k := 0; k <= 6; k++ {
			got := boundedEditDistance(a, b, k)
			if full <= k && got != full {
				t.Fatalf("bounded(%v,%v,%d)=%d, full=%d", a, b, k, got, full)
			}
			if full > k && got != -1 {
				t.Fatalf("bounded(%v,%v,%d)=%d, want -1 (full=%d)", a, b, k, got, full)
			}
		}
	}
}

package dedup

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

// bruteForce 是独立的参考实现：逐对精确评分，并用 BFS 构建重复簇。
// 判定使用独立书写的精确有理数比较，与被测实现走不同代码路径。
func bruteForce(records []Record, threshold float64) *Result {
	res := &Result{Threshold: threshold}
	adj := map[int][]int{}
	for i := 0; i < len(records); i++ {
		for j := i + 1; j < len(records); j++ {
			a, b := records[i], records[j]
			na, nb := []rune(a.Name), []rune(b.Name)
			aa, ab := []rune(a.Address), []rune(b.Address)
			dn := editDistance(na, nb)
			da := editDistance(aa, ab)
			Ln := max(len(na), len(nb))
			La := max(len(aa), len(ab))
			lhs := float64(100*((Ln-dn)*La+(La-da)*Ln)) / float64(2*Ln*La)
			if lhs < threshold {
				continue
			}
			nameSim := fieldSimilarity(dn, Ln)
			addrSim := fieldSimilarity(da, La)
			res.Pairs = append(res.Pairs, Pair{
				ID1:               a.ID,
				ID2:               b.ID,
				NameSimilarity:    nameSim,
				AddressSimilarity: addrSim,
				Score:             combinedScore(nameSim, addrSim),
			})
			adj[i] = append(adj[i], j)
			adj[j] = append(adj[j], i)
		}
	}
	sort.Slice(res.Pairs, func(x, y int) bool {
		if res.Pairs[x].ID1 != res.Pairs[y].ID1 {
			return res.Pairs[x].ID1 < res.Pairs[y].ID1
		}
		return res.Pairs[x].ID2 < res.Pairs[y].ID2
	})
	seen := make([]bool, len(records))
	for s := 0; s < len(records); s++ {
		if seen[s] || len(adj[s]) == 0 {
			continue
		}
		var cluster []string
		queue := []int{s}
		seen[s] = true
		for len(queue) > 0 {
			u := queue[0]
			queue = queue[1:]
			cluster = append(cluster, records[u].ID)
			for _, v := range adj[u] {
				if !seen[v] {
					seen[v] = true
					queue = append(queue, v)
				}
			}
		}
		sort.Strings(cluster)
		res.Clusters = append(res.Clusters, cluster)
	}
	sort.Slice(res.Clusters, func(x, y int) bool {
		return res.Clusters[x][0] < res.Clusters[y][0]
	})
	return res
}

func assertSameResult(t *testing.T, got, want *Result) {
	t.Helper()
	if len(got.Pairs) != len(want.Pairs) {
		t.Fatalf("pair count: got %d, want %d", len(got.Pairs), len(want.Pairs))
	}
	for k := range got.Pairs {
		g, w := got.Pairs[k], want.Pairs[k]
		if g.ID1 != w.ID1 || g.ID2 != w.ID2 {
			t.Fatalf("pair %d: got (%s,%s), want (%s,%s)", k, g.ID1, g.ID2, w.ID1, w.ID2)
		}
		if g.Score != w.Score || g.NameSimilarity != w.NameSimilarity || g.AddressSimilarity != w.AddressSimilarity {
			t.Fatalf("pair (%s,%s): got score %v (%v,%v), want %v (%v,%v)",
				g.ID1, g.ID2, g.Score, g.NameSimilarity, g.AddressSimilarity,
				w.Score, w.NameSimilarity, w.AddressSimilarity)
		}
	}
	if len(got.Clusters) != len(want.Clusters) {
		t.Fatalf("cluster count: got %d, want %d", len(got.Clusters), len(want.Clusters))
	}
	for k := range got.Clusters {
		if !equalStrings(got.Clusters[k], want.Clusters[k]) {
			t.Fatalf("cluster %d: got %v, want %v", k, got.Clusters[k], want.Clusters[k])
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSimilarityAndScore(t *testing.T) {
	if got := Similarity("", ""); got != 100 {
		t.Errorf("Similarity(empty,empty)=%v, want 100", got)
	}
	if got := Similarity("abc", "abc"); got != 100 {
		t.Errorf("Similarity(equal)=%v, want 100", got)
	}
	if got := Similarity("abcd", "ab"); got != 50 {
		t.Errorf("Similarity(abcd,ab)=%v, want 50", got)
	}
	// 按码点而非字节：4 个汉字差 1 个，相似度 75。
	if got := Similarity("你好世界", "你好世"); got != 75 {
		t.Errorf("Similarity(CJK)=%v, want 75", got)
	}
	p := ScorePair(Record{ID: "1", Name: "abcdef", Address: "xyz"},
		Record{ID: "2", Name: "abcdef", Address: "xyz"})
	if p.Score != 100 {
		t.Errorf("ScorePair(identical)=%v, want 100", p.Score)
	}
}

func TestThresholdBoundary(t *testing.T) {
	// 严格的 20 码点字符串，前 d 位替换为原文不存在的字符即距离为 d。
	name20 := []rune("北京朝阳创新科技发展有限公司总部大厦东区")
	addr20 := []rune("一路二街三坊四弄五村六组七号楼八单元九层")
	if len(name20) != 20 || len(addr20) != 20 {
		t.Fatalf("test data must be 20 runes, got %d/%d", len(name20), len(addr20))
	}
	vary := func(base []rune, d int, sub rune) string {
		b := append([]rune(nil), base...)
		for k := 0; k < d; k++ {
			b[k] = sub
		}
		return string(b)
	}
	nameFor := func(d int) (string, string) {
		return string(name20), vary(name20, d, '深')
	}
	addrFor := func(d int) (string, string) {
		return string(addr20), vary(addr20, d, '巷')
	}
	// 名称完全相同(100)，地址 20 字差 6(70)：综合恰好 85，必须命中。
	a1, b1 := addrFor(6)
	exact := []Record{
		{ID: "A", Name: string(name20), Address: a1},
		{ID: "B", Name: string(name20), Address: b1},
	}
	res := FindDuplicates(exact)
	if len(res.Pairs) != 1 || res.Pairs[0].Score != 85 {
		t.Fatalf("exact-85 pair must match, got %+v", res.Pairs)
	}
	// 地址差 7(65)：综合 82.5，不得命中。
	a2, b2 := addrFor(7)
	below := []Record{
		{ID: "A", Name: string(name20), Address: a2},
		{ID: "B", Name: string(name20), Address: b2},
	}
	if res := FindDuplicates(below); len(res.Pairs) != 0 {
		t.Fatalf("below-threshold pair must not match, got %+v", res.Pairs)
	}
	// 两列都不完全相等：名称 20 字差 2(90)，地址 20 字差 4(80)，综合 85。
	na, nb := nameFor(2)
	aa, ab := addrFor(4)
	both := []Record{
		{ID: "A", Name: na, Address: aa},
		{ID: "B", Name: nb, Address: ab},
	}
	res = FindDuplicates(both)
	if len(res.Pairs) != 1 || res.Pairs[0].Score != 85 {
		t.Fatalf("both-fields-fuzzy exact-85 pair must match, got %+v", res.Pairs)
	}
	if res.Pairs[0].NameSimilarity == 100 || res.Pairs[0].AddressSimilarity == 100 {
		t.Fatalf("expected both fields fuzzy, got %+v", res.Pairs[0])
	}
	// 名称 90、地址 75：综合 82.5，不得命中。
	na2, nb2 := nameFor(2)
	aa2, ab2 := addrFor(5)
	below2 := []Record{
		{ID: "A", Name: na2, Address: aa2},
		{ID: "B", Name: nb2, Address: ab2},
	}
	if res := FindDuplicates(below2); len(res.Pairs) != 0 {
		t.Fatalf("below-threshold pair must not match, got %+v", res.Pairs)
	}
}

func TestChainCluster(t *testing.T) {
	// 名称 20 字：AB 差 3(85)、BC 差 4(80)、AC 差 7(65)；地址完全相同。
	// AB、BC 达标（92.5、90），AC 不达标（82.5），但三者同属一簇。
	a := []rune("北京朝阳创新科技发展有限公司总部大厦东区")
	b := append([]rune(nil), a...)
	b[0], b[1], b[2] = '深', '深', '深'
	c := append([]rune(nil), b...)
	c[3], c[4], c[5], c[6] = '海', '海', '海', '海'
	records := []Record{
		{ID: "A", Name: string(a), Address: "上海市浦东新区世纪大道100号"},
		{ID: "B", Name: string(b), Address: "上海市浦东新区世纪大道100号"},
		{ID: "C", Name: string(c), Address: "上海市浦东新区世纪大道100号"},
	}
	res := FindDuplicates(records)
	if len(res.Pairs) != 2 {
		t.Fatalf("want 2 pairs (AB,BC), got %+v", res.Pairs)
	}
	for _, p := range res.Pairs {
		if (p.ID1 == "A" && p.ID2 == "C") || (p.ID1 == "C" && p.ID2 == "A") {
			t.Fatalf("AC must not be a candidate pair: %+v", p)
		}
	}
	if len(res.Clusters) != 1 || !equalStrings(res.Clusters[0], []string{"A", "B", "C"}) {
		t.Fatalf("want single cluster {A,B,C}, got %+v", res.Clusters)
	}
}

func TestEmptyAndSingleton(t *testing.T) {
	if res := FindDuplicates(nil); len(res.Pairs) != 0 || len(res.Clusters) != 0 {
		t.Fatalf("empty input: got %+v", res)
	}
	one := []Record{{ID: "x", Name: "甲", Address: "乙"}}
	if res := FindDuplicates(one); len(res.Pairs) != 0 || len(res.Clusters) != 0 {
		t.Fatalf("singleton input: got %+v", res)
	}
}

// randRecords 生成含近似重复的小批记录（含 CJK 与 emoji 字符集）。
func randRecords(rng *rand.Rand, n int) []Record {
	alphabets := [][]rune{
		[]rune("abcde"),
		[]rune("北京上海广州深圳"),
		[]rune("🙂🙃😀😃"),
	}
	records := make([]Record, 0, n)
	for i := 0; i < n; i++ {
		alpha := alphabets[rng.Intn(len(alphabets))]
		mk := func() string {
			l := 5 + rng.Intn(20)
			s := make([]rune, l)
			for k := range s {
				s[k] = alpha[rng.Intn(len(alpha))]
			}
			return string(s)
		}
		name, addr := mk(), mk()
		// 一半概率基于已有记录做小幅变异，制造达标对。
		if len(records) > 0 && rng.Intn(2) == 0 {
			base := records[rng.Intn(len(records))]
			mutate := func(s string) string {
				r := []rune(s)
				edits := rng.Intn(3)
				for e := 0; e < edits && len(r) > 0; e++ {
					r[rng.Intn(len(r))] = alpha[rng.Intn(len(alpha))]
				}
				return string(r)
			}
			name, addr = mutate(base.Name), mutate(base.Address)
		}
		records = append(records, Record{
			ID:      fmt.Sprintf("r%04d", i),
			Name:    name,
			Address: addr,
		})
	}
	return records
}

func TestRandomSmallVsBruteForce(t *testing.T) {
	for _, threshold := range []float64{DefaultThreshold, 80, 90, 95} {
		for seed := int64(0); seed < 8; seed++ {
			rng := rand.New(rand.NewSource(seed))
			records := randRecords(rng, 60+rng.Intn(40))
			got := FindDuplicatesWithThreshold(records, threshold)
			want := bruteForce(records, threshold)
			assertSameResult(t, got, want)
		}
	}
}

func TestShuffleDeterminism(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	records := randRecords(rng, 120)
	base := FindDuplicates(records)
	for trial := 0; trial < 5; trial++ {
		shuffled := append([]Record(nil), records...)
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		got := FindDuplicates(shuffled)
		assertSameResult(t, got, base)
	}
}

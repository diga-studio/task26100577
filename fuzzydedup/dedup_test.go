package fuzzydedup

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

// bruteForce 是独立的逐对精确评分参考实现。
func bruteForce(records []Record, threshold float64) (map[[2]string]float64, map[string][]string) {
	pairs := make(map[[2]string]float64)
	adj := make(map[string][]string)
	for i := 0; i < len(records); i++ {
		for j := i + 1; j < len(records); j++ {
			s := ScorePair(records[i], records[j])
			if s.Total >= threshold {
				a, b := records[i].ID, records[j].ID
				if b < a {
					a, b = b, a
				}
				pairs[[2]string{a, b}] = s.Total
				adj[a] = append(adj[a], b)
				adj[b] = append(adj[b], a)
			}
		}
	}
	return pairs, adj
}

func pairSet(pairs []Pair) map[[2]string]float64 {
	m := make(map[[2]string]float64, len(pairs))
	for _, p := range pairs {
		m[[2]string{p.ID1, p.ID2}] = p.Score
	}
	return m
}

// 小批随机数据：库结果必须与逐对精确评分的参考结果完全一致。
func TestDeduplicateMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	names := []string{"北京科技公司", "北京科枝公司", "上海贸易集团", "上海贸昜集团", "广州食品厂", "深圳电子城", "杭州网络科技", "成都餐饮管理"}
	addrs := []string{"北京市海淀区中关村大街1号", "北京市海淀区中关村大街2号", "上海市浦东新区世纪大道100号", "上海市浦东新区世纪大道101号", "广州市天河区体育西路5号", "深圳市南山区科技园南路8号"}
	for trial := 0; trial < 30; trial++ {
		n := 10 + rng.Intn(40)
		records := make([]Record, n)
		for i := range records {
			records[i] = Record{
				ID:      fmt.Sprintf("R%03d", i),
				Name:    names[rng.Intn(len(names))],
				Address: addrs[rng.Intn(len(addrs))],
			}
		}
		got := Deduplicate(records)
		wantPairs, _ := bruteForce(records, DefaultThreshold)
		gotPairs := pairSet(got.Pairs)
		if !reflect.DeepEqual(gotPairs, wantPairs) {
			t.Fatalf("trial %d: pairs mismatch\ngot:  %v\nwant: %v", trial, gotPairs, wantPairs)
		}
	}
}

// 综合分数恰好 85 必须命中；两侧边界都要覆盖。
func TestThresholdBoundary(t *testing.T) {
	mk := func(id, name, addr string) Record { return Record{ID: id, Name: name, Address: addr} }
	name10 := "abcdefghij" // 10 码点
	addr10 := "ABCDEFGHIJ" // 10 码点
	nameD2 := "ab--efghij" // 与 name10 距离 2 -> 80
	nameD1 := "abcdefghix" // 与 name10 距离 1 -> 90
	addrD1 := "ABCDEFGHI-" // 与 addr10 距离 1 -> 90
	addrD2 := "ABCD--GHIJ" // 与 addr10 距离 2 -> 80
	addrD3 := "ABCDEFG---" // 与 addr10 距离 3 -> 70
	nameD3 := "abc---ghij" // 与 name10 距离 3 -> 70

	cases := []struct {
		desc    string
		a, b    Record
		wantHit bool
	}{
		{"两列均不完全相等仍达标(80+90)/2=85", mk("A", nameD2, addr10), mk("B", name10, addrD1), true},
		{"恰好85(90+80)/2=85", mk("A", nameD1, addr10), mk("B", name10, addrD2), true},
		{"恰好85(100+70)/2=85", mk("A", name10, addrD3), mk("B", name10, addr10), true},
		{"低于阈值(80+80)/2=80", mk("A", nameD2, addr10), mk("B", name10, addrD2), false},
		{"低于阈值(90+70)/2=80", mk("A", nameD1, addrD3), mk("B", name10, addr10), false},
		{"单侧低于85仍可达标(70+100)/2=85", mk("A", nameD3, addr10), mk("B", name10, addr10), true},
	}
	for _, c := range cases {
		res := Deduplicate([]Record{c.a, c.b})
		hit := len(res.Pairs) == 1
		if hit != c.wantHit {
			s := ScorePair(c.a, c.b)
			t.Errorf("%s: hit=%v want %v (name=%v addr=%v total=%v)",
				c.desc, hit, c.wantHit, s.NameSimilarity, s.AddressSimilarity, s.Total)
		}
		if hit {
			if res.Pairs[0].ID1 != "A" || res.Pairs[0].ID2 != "B" {
				t.Errorf("%s: pair IDs = %v,%v", c.desc, res.Pairs[0].ID1, res.Pairs[0].ID2)
			}
			if len(res.Clusters) != 1 || !reflect.DeepEqual(res.Clusters[0], []string{"A", "B"}) {
				t.Errorf("%s: clusters = %v", c.desc, res.Clusters)
			}
		}
	}
}

// Unicode 文本（多字节码点）参与查重。
func TestUnicodeRecords(t *testing.T) {
	records := []Record{
		{ID: "1", Name: "東京タワー株式会社", Address: "東京都港区芝公園4丁目"},
		{ID: "2", Name: "東京タワー株式会杜", Address: "東京都港区芝公園4丁目"},
		{ID: "3", Name: "完全不一样的公司🙂", Address: "另外一个城市的地址"},
	}
	res := Deduplicate(records)
	if len(res.Pairs) != 1 || res.Pairs[0].ID1 != "1" || res.Pairs[0].ID2 != "2" {
		t.Fatalf("pairs = %+v", res.Pairs)
	}
	if len(res.Clusters) != 1 || !reflect.DeepEqual(res.Clusters[0], []string{"1", "2"}) {
		t.Fatalf("clusters = %v", res.Clusters)
	}
}

// 链式相似：A-B、B-C 达标，A-C 不达标；三者同簇，但候选对不能虚构 A-C。
func TestChainCluster(t *testing.T) {
	base := "ABCDEFGHIJ" // 10 码点
	mk := func(id, addr string) Record {
		return Record{ID: id, Name: "同名公司", Address: addr}
	}
	a := mk("A", base)
	b := mk("B", "AB--EFGHIJ") // 与 A 距离 2 -> 80，综合 (100+80)/2=90 达标
	c := mk("C", "AB--EFGH--") // 与 B 距离 2 -> 80 达标；与 A 距离 4 -> 60，综合 80 不达标
	d := mk("D", "ZZZZZZZZZZ") // 孤立记录

	res := Deduplicate([]Record{a, b, c, d})
	got := pairSet(res.Pairs)
	if len(got) != 2 {
		t.Fatalf("pairs = %v, 应只有 A-B 与 B-C", got)
	}
	if _, ok := got[[2]string{"A", "C"}]; ok {
		t.Fatal("候选对中不能虚构 A-C")
	}
	if len(res.Clusters) != 1 || !reflect.DeepEqual(res.Clusters[0], []string{"A", "B", "C"}) {
		t.Fatalf("clusters = %v, 应为 [A B C]", res.Clusters)
	}
}

// 输入重排后，候选对集合与簇成员不变。
func TestShuffleInvariance(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	names := []string{"北京科技公司", "北京科枝公司", "上海贸易集团", "上海贸昜集团", "广州食品厂"}
	addrs := []string{"北京市海淀区中关村大街1号", "北京市海淀区中关村大街2号", "上海市浦东新区世纪大道100号", "广州市天河区体育西路5号"}
	base := make([]Record, 40)
	for i := range base {
		base[i] = Record{
			ID:      fmt.Sprintf("R%03d", i),
			Name:    names[rng.Intn(len(names))],
			Address: addrs[rng.Intn(len(addrs))],
		}
	}
	want := Deduplicate(base)
	for trial := 0; trial < 20; trial++ {
		shuffled := append([]Record(nil), base...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		got := Deduplicate(shuffled)
		if !reflect.DeepEqual(got.Pairs, want.Pairs) {
			t.Fatalf("trial %d: pairs differ after shuffle", trial)
		}
		if !reflect.DeepEqual(got.Clusters, want.Clusters) {
			t.Fatalf("trial %d: clusters differ after shuffle", trial)
		}
	}
}

// 同一 ID 不与自身比较，同一对只返回一次，孤立记录不进簇。
func TestNoSelfPairAndIsolated(t *testing.T) {
	records := []Record{
		{ID: "X", Name: "公司甲", Address: "地址一"},
		{ID: "Y", Name: "公司甲", Address: "地址一"},
		{ID: "Z", Name: "完全不同", Address: "毫无关系"},
	}
	res := Deduplicate(records)
	if len(res.Pairs) != 1 {
		t.Fatalf("pairs = %+v", res.Pairs)
	}
	p := res.Pairs[0]
	if p.ID1 == p.ID2 {
		t.Fatal("出现自身配对")
	}
	if len(res.Clusters) != 1 || !reflect.DeepEqual(res.Clusters[0], []string{"X", "Y"}) {
		t.Fatalf("clusters = %v", res.Clusters)
	}
}

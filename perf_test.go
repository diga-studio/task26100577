package dedup

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"
)

// 测试字符集取自 CJK 统一表意文字连续区段，规模贴近真实中文姓名地址用字。
var nameAlphabet = cjkAlphabet(0x4e00, 400)
var addrAlphabet = cjkAlphabet(0x5200, 300)

func cjkAlphabet(start rune, size int) []rune {
	a := make([]rune, size)
	for i := range a {
		a[i] = start + rune(i)
	}
	return a
}

// randString 生成长度在 [minLen,maxLen] 码点的随机字符串。
func randString(rng *rand.Rand, alphabet []rune, minLen, maxLen int) string {
	l := minLen + rng.Intn(maxLen-minLen+1)
	s := make([]rune, l)
	for i := range s {
		s[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(s)
}

// mutate 对字符串做 k 次替换式错字。
func mutate(rng *rand.Rand, s string, alphabet []rune, k int) string {
	r := []rune(s)
	for e := 0; e < k && len(r) > 0; e++ {
		pos := rng.Intn(len(r))
		nr := alphabet[rng.Intn(len(alphabet))]
		if nr != r[pos] {
			r[pos] = nr
		}
	}
	return string(r)
}

// buildSameNameBatch 构造"大量同名、地址分散"的 10000 条批次：
// 若干大组与小组共享完全相同的名称，地址随机分散；
// 另植入 plantedPairs 对名称相同且地址仅差 2 字的达标对。
func buildSameNameBatch(seed int64, plantedPairs int) []Record {
	rng := rand.New(rand.NewSource(seed))
	records := make([]Record, 0, 10000)
	addGroup := func(size int) {
		name := randString(rng, nameAlphabet, 20, 40)
		for k := 0; k < size; k++ {
			records = append(records, Record{
				ID:      fmt.Sprintf("r%05d", len(records)),
				Name:    name,
				Address: randString(rng, addrAlphabet, 30, 60),
			})
		}
	}
	for len(records) < 5000 {
		addGroup(100) // 50 个 100 人大组
	}
	for len(records) < 10000 {
		addGroup(5) // 1000 个 5 人小组
	}
	// 植入达标对：每组前两条记录地址仅差 2 字。
	for p := 0; p < plantedPairs; p++ {
		base := records[p*10].Address
		records[p*10+1].Address = mutate(rng, base, addrAlphabet, 2)
		records[p*10+1].Address = string(append([]rune(nil), []rune(records[p*10+1].Address)...))
	}
	return records
}

// buildTypoBatch 构造"名称和地址都存在小幅错字"的 10000 条批次：
// 1000 个实体各 10 条变体，每条变体在名称与地址上各有 1-2 处错字，
// 组内任意两条均达标，共 1000*C(10,2)=45000 对。
func buildTypoBatch(seed int64) []Record {
	rng := rand.New(rand.NewSource(seed))
	records := make([]Record, 0, 10000)
	for g := 0; g < 1000; g++ {
		name := randString(rng, nameAlphabet, 40, 50)
		addr := randString(rng, addrAlphabet, 40, 60)
		for k := 0; k < 10; k++ {
			records = append(records, Record{
				ID:      fmt.Sprintf("r%05d", len(records)),
				Name:    mutate(rng, name, nameAlphabet, 1+rng.Intn(2)),
				Address: mutate(rng, addr, addrAlphabet, 1+rng.Intn(2)),
			})
		}
	}
	return records
}

// memStats 返回当前堆分配与累计分配（MiB）。
func memStats() (heapAlloc, totalAlloc float64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	const mib = 1024 * 1024
	return float64(m.HeapAlloc) / mib, float64(m.TotalAlloc) / mib
}

// runPerfScenario 执行一次 10000 条批次的性能测试并校验结果不变量。
func runPerfScenario(t *testing.T, name string, records []Record,
	wantPairs, wantClusters int) (elapsed time.Duration, peakHeap, total float64) {
	t.Helper()
	if len(records) != 10000 {
		t.Fatalf("%s: want 10000 records, got %d", name, len(records))
	}
	runtime.GC()
	heapBefore, totalBefore := memStats()
	start := time.Now()
	res := FindDuplicates(records)
	elapsed = time.Since(start)
	runtime.GC()
	heapAfter, totalAfter := memStats()
	peakHeap = heapAfter - heapBefore
	total = totalAfter - totalBefore
	t.Logf("%s: 耗时 %v，结果堆内存 %.1f MiB，累计分配 %.1f MiB，达标对 %d，簇 %d",
		name, elapsed, peakHeap, total, len(res.Pairs), len(res.Clusters))
	if len(res.Pairs) != wantPairs {
		t.Fatalf("%s: got %d pairs, want %d", name, len(res.Pairs), wantPairs)
	}
	if len(res.Clusters) != wantClusters {
		t.Fatalf("%s: got %d clusters, want %d", name, len(res.Clusters), wantClusters)
	}
	if elapsed > 20*time.Second {
		t.Fatalf("%s: elapsed %v exceeds 20s budget", name, elapsed)
	}
	if total > 256 {
		t.Fatalf("%s: total allocation %.1f MiB exceeds 256 MiB budget", name, total)
	}
	// 抽样校验：前 200 条记录子集与暴力参考结果一致。
	sample := append([]Record(nil), records[:200]...)
	want := bruteForce(sample, DefaultThreshold)
	inSample := map[string]bool{}
	for _, r := range sample {
		inSample[r.ID] = true
	}
	gotCount := 0
	for _, p := range res.Pairs {
		if inSample[p.ID1] && inSample[p.ID2] {
			gotCount++
		}
	}
	if gotCount != len(want.Pairs) {
		t.Fatalf("%s: sample pairs got %d, want %d", name, gotCount, len(want.Pairs))
	}
	return elapsed, peakHeap, total
}

func TestPerfSameNameScatteredAddress(t *testing.T) {
	records := buildSameNameBatch(20261005, 500)
	runPerfScenario(t, "同名地址分散", records, 500, 500)
}

func TestPerfTypoBatch(t *testing.T) {
	records := buildTypoBatch(20261005)
	runPerfScenario(t, "双字段错字", records, 45000, 1000)
}

func BenchmarkFindDuplicatesSameName(b *testing.B) {
	records := buildSameNameBatch(20261005, 500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FindDuplicates(records)
	}
}

func BenchmarkFindDuplicatesTypoBatch(b *testing.B) {
	records := buildTypoBatch(20261005)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FindDuplicates(records)
	}
}

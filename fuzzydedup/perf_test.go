package fuzzydedup

import (
	"fmt"
	"math/rand"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// ---------- 可复现的数据生成器（固定随机种子） ----------

var (
	cityPool   = []string{"北京", "上海", "广州", "深圳", "杭州", "成都", "武汉", "西安", "南京", "重庆", "苏州", "天津"}
	roadPool   = []string{"中山", "解放", "建设", "人民", "和平", "胜利", "文化", "青年", "朝阳", "滨江", "学府", "创业"}
	namePrefix = []string{"华", "中", "天", "金", "瑞", "恒", "博", "联", "宏", "泰", "鑫", "正"}
	nameMid    = []string{"宇", "诚", "达", "科", "盛", "源", "兴", "汇", "嘉", "德", "立", "同"}
	nameSuffix = []string{"科技有限公司", "贸易有限公司", "实业集团", "控股集团", "网络技术公司", "食品有限公司"}
	extraChars = []rune("东南西北新旧港湖山林园景春秋天地人和丰润安泰")
)

func randName(rng *rand.Rand) string {
	n := namePrefix[rng.Intn(len(namePrefix))] +
		nameMid[rng.Intn(len(nameMid))] +
		nameMid[rng.Intn(len(nameMid))] +
		nameSuffix[rng.Intn(len(nameSuffix))]
	for len([]rune(n)) < 20 {
		n += string(extraChars[rng.Intn(len(extraChars))])
	}
	return n
}

func randAddress(rng *rand.Rand) string {
	a := cityPool[rng.Intn(len(cityPool))] + "市" +
		roadPool[rng.Intn(len(roadPool))] + "区" +
		roadPool[rng.Intn(len(roadPool))] + "路" +
		fmt.Sprintf("%d号", 1+rng.Intn(1999)) +
		fmt.Sprintf("%d栋", 1+rng.Intn(50)) +
		fmt.Sprintf("%d室", 1+rng.Intn(3000))
	for len([]rune(a)) < 20 {
		a += string(extraChars[rng.Intn(len(extraChars))])
	}
	return a
}

// genSameNameBatch 生成大量同名但地址分散的批次：40 个公司名，每个约 250 条。
func genSameNameBatch(seed int64, n int) []Record {
	rng := rand.New(rand.NewSource(seed))
	names := make([]string, 40)
	for i := range names {
		names[i] = randName(rng)
	}
	records := make([]Record, n)
	for i := range records {
		records[i] = Record{
			ID:      fmt.Sprintf("R%06d", i),
			Name:    names[i%len(names)],
			Address: randAddress(rng),
		}
	}
	return records
}

// mutate 对字符串做 k 次随机编辑（替换/插入/删除）。
func mutate(rng *rand.Rand, s string, k int) string {
	r := []rune(s)
	for ; k > 0 && len(r) > 1; k-- {
		pos := rng.Intn(len(r))
		switch rng.Intn(3) {
		case 0:
			r[pos] = extraChars[rng.Intn(len(extraChars))]
		case 1:
			r = append(r[:pos], r[pos+1:]...)
		case 2:
			r = append(r[:pos], append([]rune{extraChars[rng.Intn(len(extraChars))]}, r[pos:]...)...)
		}
	}
	return string(r)
}

// genTypoBatch 生成名称和地址都存在小幅错字的批次：
// 约 70% 为互不相同的基础记录，30% 为基础记录的错字副本。
func genTypoBatch(seed int64, n int) []Record {
	rng := rand.New(rand.NewSource(seed))
	baseCount := n * 7 / 10
	records := make([]Record, 0, n)
	for i := 0; i < baseCount; i++ {
		records = append(records, Record{
			ID:      fmt.Sprintf("R%06d", len(records)),
			Name:    randName(rng),
			Address: randAddress(rng),
		})
	}
	for len(records) < n {
		base := records[rng.Intn(baseCount)]
		records = append(records, Record{
			ID:      fmt.Sprintf("R%06d", len(records)),
			Name:    mutate(rng, base.Name, 1+rng.Intn(3)),
			Address: mutate(rng, base.Address, 1+rng.Intn(3)),
		})
	}
	return records
}

// ---------- 资源测量 ----------

func cpuTime() time.Duration {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	sec := ru.Utime.Sec + ru.Stime.Sec
	usec := int64(ru.Utime.Usec) + int64(ru.Stime.Usec)
	return time.Duration(sec)*time.Second + time.Duration(usec)*time.Microsecond
}

// peakHeap 在后台采样 HeapInuse，返回停止后给出峰值的通道。
func peakHeap(stop <-chan struct{}) <-chan uint64 {
	done := make(chan uint64, 1)
	go func() {
		var peak uint64
		var m runtime.MemStats
		for {
			select {
			case <-stop:
				done <- peak
				return
			default:
				runtime.ReadMemStats(&m)
				if m.HeapInuse > peak {
					peak = m.HeapInuse
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()
	return done
}

func runPerfCase(t *testing.T, desc string, records []Record) Result {
	t.Helper()
	runtime.GC()
	stop := make(chan struct{})
	peakCh := peakHeap(stop)
	cpuStart := cpuTime()
	wallStart := time.Now()

	res := Deduplicate(records)

	wall := time.Since(wallStart)
	cpu := cpuTime() - cpuStart
	close(stop)
	peak := <-peakCh

	t.Logf("%s: 记录数=%d 达标对=%d 簇数=%d", desc, len(records), len(res.Pairs), len(res.Clusters))
	t.Logf("%s: 累计CPU时间=%v 墙钟时间=%v 峰值堆内存=%.1f MiB",
		desc, cpu, wall, float64(peak)/1024/1024)
	if cpu > 20*time.Second {
		t.Errorf("%s: 累计CPU时间 %v 超过 20 秒", desc, cpu)
	}
	if peak > 256*1024*1024 {
		t.Errorf("%s: 峰值堆内存 %.1f MiB 超过 256 MiB", desc, float64(peak)/1024/1024)
	}
	return res
}

func TestPerfSameNameScatteredAddress(t *testing.T) {
	res := runPerfCase(t, "同名地址分散批次", genSameNameBatch(20261005, 10000))
	if len(res.Pairs) == 0 {
		t.Log("注意：该批次没有达标对（地址完全分散）")
	}
}

func TestPerfTypoBatch(t *testing.T) {
	res := runPerfCase(t, "错字批次", genTypoBatch(20261005, 10000))
	if len(res.Pairs) == 0 {
		t.Fatal("错字批次应产生达标对")
	}
	if len(res.Pairs) > 50000 {
		t.Fatalf("达标对 %d 超过 50000 的设计上限", len(res.Pairs))
	}
}

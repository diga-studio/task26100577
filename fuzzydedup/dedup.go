package fuzzydedup

import "sort"

// DefaultThreshold 是综合分数的达标阈值（未舍入值 >= 85 即达标）。
const DefaultThreshold = 85.0

// filterEps 是长度预过滤的容差，只可能多算、不会漏掉恰好达标的记录对。
const filterEps = 1e-9

// Record 是一条待查重记录。ID 在批次内必须唯一；
// Name 与 Address 均非空，且已由上游完成大小写及空白标准化，本库直接比较收到的文本。
type Record struct {
	ID      string
	Name    string
	Address string
}

// Pair 是一对达标记录，ID1 < ID2，所有分数均为未舍入的数学值。
type Pair struct {
	ID1               string
	ID2               string
	NameSimilarity    float64
	AddressSimilarity float64
	Score             float64
}

// Result 是整批查重的结果。
// Pairs 包含全部达标记录对，按 (ID1, ID2) 排序，同一对只出现一次。
// Clusters 只包含成员数 >= 2 的重复簇；簇内 ID 有序，簇之间按首个成员排序。
// 结果与输入记录的顺序无关。
type Result struct {
	Pairs    []Pair
	Clusters [][]string
}

// Deduplicate 对整批记录查重，使用默认阈值 85。
// 同一 ID 不与自身比较；所有综合分数达标（>= 85，未舍入）的记录对都会返回。
func Deduplicate(records []Record) Result {
	return DeduplicateWithThreshold(records, DefaultThreshold)
}

// DeduplicateWithThreshold 与 Deduplicate 相同，但使用自定义综合分数阈值。
func DeduplicateWithThreshold(records []Record, threshold float64) Result {
	n := len(records)
	ids := make([]string, n)
	names := make([]string, n)
	nameRunes := make([][]rune, n)
	addrRunes := make([][]rune, n)
	for i, r := range records {
		ids[i] = r.ID
		names[i] = r.Name
		nameRunes[i] = []rune(r.Name)
		addrRunes[i] = []rune(r.Address)
	}

	// 批级符号编码：把码点映射为紧凑整数，为每条记录的每个字段
	// 预计算 Myers pattern 侧的 eq 表，避免逐对重复构建。
	nameSyms, encNames := encodeFields(nameRunes)
	addrSyms, encAddrs := encodeFields(addrRunes)
	nameProfiles := make([]eqProfile, n)
	addrProfiles := make([]eqProfile, n)
	for i := 0; i < n; i++ {
		nameProfiles[i] = newEqProfile(encNames[i], nameSyms)
		addrProfiles[i] = newEqProfile(encAddrs[i], addrSyms)
	}

	need := 2 * threshold
	var pairs []Pair
	uf := newUnionFind(n)

	for i := 0; i < n; i++ {
		ni, ai := nameRunes[i], addrRunes[i]
		lni, lai := len(ni), len(ai)
		for j := i + 1; j < n; j++ {
			// 长度预过滤：编辑距离不小于长度差，给出两字段相似度的上界。
			maxName := boundSim(lni, len(nameRunes[j]))
			maxAddr := boundSim(lai, len(addrRunes[j]))
			if maxName+maxAddr < need-filterEps {
				continue
			}
			// 名称完全相同的快捷路径（大量同名批次的关键优化）。
			var nameSim float64
			if names[i] == names[j] {
				nameSim = 100
			} else {
				nameSim = fieldSimEncoded(nameProfiles, encNames, nameRunes, i, j)
			}
			if nameSim+maxAddr < need-filterEps {
				continue
			}
			addrSim := fieldSimEncoded(addrProfiles, encAddrs, addrRunes, i, j)
			total := (nameSim + addrSim) / 2
			if total < threshold {
				continue
			}
			id1, id2 := ids[i], ids[j]
			if id2 < id1 {
				id1, id2 = id2, id1
			}
			pairs = append(pairs, Pair{
				ID1:               id1,
				ID2:               id2,
				NameSimilarity:    nameSim,
				AddressSimilarity: addrSim,
				Score:             total,
			})
			uf.union(i, j)
		}
	}

	sort.Slice(pairs, func(a, b int) bool {
		if pairs[a].ID1 != pairs[b].ID1 {
			return pairs[a].ID1 < pairs[b].ID1
		}
		return pairs[a].ID2 < pairs[b].ID2
	})

	return Result{Pairs: pairs, Clusters: buildClusters(uf, ids)}
}

// encodeFields 把整批字段的码点切片编码为符号整数，返回符号表大小与编码结果。
func encodeFields(fields [][]rune) (int, [][]uint16) {
	symOf := make(map[rune]uint16)
	encoded := make([][]uint16, len(fields))
	for i, f := range fields {
		enc := make([]uint16, len(f))
		for k, r := range f {
			sym, ok := symOf[r]
			if !ok {
				sym = uint16(len(symOf))
				symOf[r] = sym
			}
			enc[k] = sym
		}
		encoded[i] = enc
	}
	return len(symOf), encoded
}

// fieldSimEncoded 用预计算的 eq 表计算两条记录某字段的相似度。
// 码点长度超过 64 的长字段回退到动态规划。
func fieldSimEncoded(profiles []eqProfile, encoded [][]uint16, runes [][]rune, i, j int) float64 {
	li, lj := len(runes[i]), len(runes[j])
	longer := li
	if lj > longer {
		longer = lj
	}
	if longer == 0 {
		return 100
	}
	// pattern 取较短的一方，保证 Myers 位掩码尽量放得下。
	p, text := i, j
	if li > lj {
		p, text = j, i
	}
	var d int
	if profiles[p].length <= 64 {
		d = myersDense(profiles[p], encoded[text])
	} else {
		d = dpLevenshtein(runes[p], runes[text])
	}
	return (1 - float64(d)/float64(longer)) * 100
}

// boundSim 由字段长度给出相似度上界：(1 - 长度差/较长长度) * 100。
func boundSim(la, lb int) float64 {
	longer := la
	if lb > longer {
		longer = lb
	}
	if longer == 0 {
		return 100
	}
	diff := la - lb
	if diff < 0 {
		diff = -diff
	}
	return (1 - float64(diff)/float64(longer)) * 100
}

// buildClusters 把并查集中的连通分量转成排序后的重复簇，孤立记录不进入结果。
func buildClusters(uf *unionFind, ids []string) [][]string {
	groups := make(map[int][]int)
	for i := range ids {
		root := uf.find(i)
		groups[root] = append(groups[root], i)
	}
	clusters := make([][]string, 0, len(groups))
	for _, members := range groups {
		if len(members) < 2 {
			continue
		}
		cluster := make([]string, len(members))
		for k, idx := range members {
			cluster[k] = ids[idx]
		}
		sort.Strings(cluster)
		clusters = append(clusters, cluster)
	}
	sort.Slice(clusters, func(a, b int) bool {
		return clusters[a][0] < clusters[b][0]
	})
	return clusters
}

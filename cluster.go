package dedup

import "sort"

// unionFind 是用于构建重复簇的并查集，带路径压缩与按秩合并。
type unionFind struct {
	parent []int
	rank   []int
}

func newUnionFind(n int) *unionFind {
	u := &unionFind{parent: make([]int, n), rank: make([]int, n)}
	for i := range u.parent {
		u.parent[i] = i
	}
	return u
}

func (u *unionFind) find(x int) int {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return
	}
	if u.rank[ra] < u.rank[rb] {
		ra, rb = rb, ra
	}
	u.parent[rb] = ra
	if u.rank[ra] == u.rank[rb] {
		u.rank[ra]++
	}
}

// buildClusters 根据达标记录对（记录下标）构建重复簇。
// 返回的每个簇含至少两个成员，簇内 ID 升序，簇间按首个 ID 升序，
// 因此结果与输入顺序无关。孤立记录不出现在任何簇中。
func buildClusters(recs []recordData, pairs [][2]int) [][]string {
	u := newUnionFind(len(recs))
	for _, p := range pairs {
		u.union(p[0], p[1])
	}
	groups := map[int][]string{}
	for i := range recs {
		root := u.find(i)
		groups[root] = append(groups[root], recs[i].id)
	}
	clusters := make([][]string, 0, len(groups))
	for _, members := range groups {
		if len(members) < 2 {
			continue
		}
		sort.Strings(members)
		clusters = append(clusters, members)
	}
	sort.Slice(clusters, func(a, b int) bool {
		return clusters[a][0] < clusters[b][0]
	})
	return clusters
}

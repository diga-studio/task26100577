package fuzzydedup

// unionFind 是基于路径压缩与按秩合并的并查集，用于把达标记录对聚成重复簇。
type unionFind struct {
	parent []int
	rank   []int
}

func newUnionFind(n int) *unionFind {
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	return &unionFind{parent: parent, rank: make([]int, n)}
}

func (u *unionFind) find(x int) int {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(x, y int) {
	rx, ry := u.find(x), u.find(y)
	if rx == ry {
		return
	}
	if u.rank[rx] < u.rank[ry] {
		rx, ry = ry, rx
	}
	u.parent[ry] = rx
	if u.rank[rx] == u.rank[ry] {
		u.rank[rx]++
	}
}

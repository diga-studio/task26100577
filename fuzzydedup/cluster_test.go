package fuzzydedup

import "testing"

func TestUnionFind(t *testing.T) {
	u := newUnionFind(5)
	u.union(0, 1)
	u.union(1, 2)
	u.union(3, 4)
	if u.find(0) != u.find(2) {
		t.Error("0 和 2 应在同一集合")
	}
	if u.find(0) == u.find(3) {
		t.Error("0 和 3 不应在同一集合")
	}
	if u.find(3) != u.find(4) {
		t.Error("3 和 4 应在同一集合")
	}
	u.union(2, 4)
	if u.find(0) != u.find(4) {
		t.Error("合并后 0 和 4 应在同一集合")
	}
}

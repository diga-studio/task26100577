# fuzzydedup — 企业数据清洗模糊查重库

用 Go 实现的进程内模糊查重组件，供上层数据清洗任务调用。
基于 Levenshtein 编辑距离对记录的名称、地址评分，找出值得人工复核的
重复记录对，并把直接或间接相连的记录组织成重复簇。
本库不负责人工合并，也不修改原始记录。

## 功能

- **单对评分**：`ScorePair` 返回两条记录的名称相似度、地址相似度与综合分数。
- **整批查重**：`Deduplicate` 返回全部达标记录对（含各对综合分数）与重复簇。
- **完整性**：所有综合分数 ≥ 85（未舍入数学值，恰好 85 必命中）的记录对都会返回，
  不做 top-k 截断，同簇记录之间的达标关系也不会省略。
- **重复簇**：通过达标对直接或间接相连的记录归入同一簇；孤立记录不进簇；
  候选对中不会虚构未达标的关系。
- **确定性**：候选对集合与簇成员与输入顺序无关；输出按 ID 规范排序。

## 业务规则

- 单字段相似度 = `(1 - 编辑距离 / 较长字段码点长度) × 100`，
  编辑距离为插入/删除/替换代价均为 1 的 Levenshtein 距离，按 Unicode 码点计算。
- 综合分数 = 名称相似度与地址相似度的算术平均值，阈值 85（可用
  `DeduplicateWithThreshold` 自定义）。不要求任何一列完全相等，
  也不要求两列各自达到 85。
- 输入记录的 ID 必须在批次内唯一；名称、地址非空且已由上游完成
  大小写及空白标准化，本库直接比较收到的文本。

## 使用

```go
import "fuzzydedup"

records := []fuzzydedup.Record{
    {ID: "C001", Name: "北京华宇科技有限公司", Address: "北京市海淀区中关村大街1号"},
    {ID: "C002", Name: "北京华宇科枝有限公司", Address: "北京市海淀区中关村大街1号"},
    {ID: "C003", Name: "上海天成贸易集团",     Address: "上海市浦东新区世纪大道100号"},
}

// 整批查重（阈值 85）
res := fuzzydedup.Deduplicate(records)
for _, p := range res.Pairs {
    fmt.Printf("%s <-> %s 综合=%.2f 名称=%.2f 地址=%.2f\n",
        p.ID1, p.ID2, p.Score, p.NameSimilarity, p.AddressSimilarity)
}
for _, cluster := range res.Clusters {
    fmt.Printf("重复簇: %v\n", cluster)
}

// 单对评分
s := fuzzydedup.ScorePair(records[0], records[1])
fmt.Println(s.NameSimilarity, s.AddressSimilarity, s.Total)

// 单字段相似度 / 编辑距离
fmt.Println(fuzzydedup.FieldSimilarity("北京中关村", "北京中关村"))
fmt.Println(fuzzydedup.Levenshtein("kitten", "sitting")) // 3
```

## 实现要点

- 编辑距离核心为 Myers 位并行算法（pattern ≤ 64 码点时 O(长度)、
  每步仅常数个 64 位运算），更长文本回退到两行动态规划。
- 批处理前对整批字段做符号编码，并为每条记录的每个字段预计算
  Myers pattern 侧 eq 表，避免逐对重复构建。
- 逐对评分前用长度差给出相似度上界做完整（不漏报）的预过滤；
  名称完全相同的记录对走快捷路径。预过滤只可能多算、不会漏掉
  恰好达标的记录对。
- 重复簇由路径压缩 + 按秩合并的并查集生成。

## 测试

```bash
cd fuzzydedup
go test ./...          # 全部正确性 + 性能测试
go test -run TestPerf -v ./...   # 只看性能用例的实际耗时与内存
```

正确性测试覆盖：与独立全矩阵 DP 参考实现的随机交叉验证、
小批结果与逐对精确评分参考结果完全一致、Unicode 文本、
恰好 85 及阈值两侧、两列均不完全相等仍达标、链式相似形成大簇
（簇内并非任意两条都达标）、输入重排后结果不变、
自身不配对 / 同一对只返回一次 / 孤立记录不进簇。

## 性能实测（可复现）

测试数据由固定随机种子生成（见 `perf_test.go`），单批 10000 条记录，
名称与地址均为 20–60 码点。运行环境：Apple Silicon（darwin/arm64），
Go 1.27，单批一次调用。

| 批次 | 特征 | 达标对 | 簇数 | 累计 CPU 时间 | 峰值堆内存 |
| --- | --- | --- | --- | --- | --- |
| 同名地址分散 | 40 个公司名 × 约 250 条，地址随机分散 | 133 | 127 | ≈3.7 s | ≈19 MiB |
| 错字批次 | 70% 互不相同基础记录 + 30% 名称地址均含 1–3 处错字的副本 | 3177 | 2431 | ≈4.8 s | ≈23 MiB |

两类批次均远低于 20 秒 CPU / 256 MiB 的预算上限。

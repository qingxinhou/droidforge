// Package cpu 读取设备 CPU 各核心的实时频率与调频信息。
package cpu

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// Exec 执行一条设备端 shell 命令。
type Exec = func(ctx context.Context, command string) (string, error)

// Core 是单个核心的状态，频率单位为 kHz。
type Core struct {
	Index    int     `json:"index"`
	Cur      int64   `json:"cur"`
	Min      int64   `json:"min"`
	Max      int64   `json:"max"`
	Governor string  `json:"governor"`
	Freqs    []int64 `json:"freqs"`
}

// Cluster 是共用同一调频策略的核心组。
type Cluster struct {
	Index    int     `json:"index"`
	Cores    []int   `json:"cores"`
	Min      int64   `json:"min"`
	Max      int64   `json:"max"`
	Cur      int64   `json:"cur"`
	Governor string  `json:"governor"`
	Freqs    []int64 `json:"freqs"`
}

// Snapshot 是一次 CPU 状态快照。
type Snapshot struct {
	Cores    []Core    `json:"cores"`
	Clusters []Cluster `json:"clusters"`
}

const script = `for d in /sys/devices/system/cpu/cpu[0-9]*; do
n=${d##*/cpu}
p=$d/cpufreq
echo "C:$n:$(cat $p/scaling_cur_freq 2>/dev/null):$(cat $p/cpuinfo_min_freq 2>/dev/null):$(cat $p/cpuinfo_max_freq 2>/dev/null):$(cat $p/scaling_governor 2>/dev/null):$(cat $p/scaling_available_frequencies 2>/dev/null)"
done`

// Read 读取所有核心的当前频率。
func Read(ctx context.Context, exec Exec) (Snapshot, error) {
	out, err := exec(ctx, script)
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{Cores: make([]Core, 0, 8)}
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "C:") {
			continue
		}
		parts := strings.SplitN(line, ":", 7)
		if len(parts) < 7 {
			continue
		}
		c := Core{
			Index:    int(parseInt(parts[1])),
			Cur:      parseInt(parts[2]),
			Min:      parseInt(parts[3]),
			Max:      parseInt(parts[4]),
			Governor: strings.TrimSpace(parts[5]),
		}
		for _, f := range strings.Fields(parts[6]) {
			if v := parseInt(f); v > 0 {
				c.Freqs = append(c.Freqs, v)
			}
		}
		sort.Slice(c.Freqs, func(i, j int) bool { return c.Freqs[i] < c.Freqs[j] })
		snap.Cores = append(snap.Cores, c)
	}
	sort.Slice(snap.Cores, func(i, j int) bool { return snap.Cores[i].Index < snap.Cores[j].Index })
	snap.Clusters = buildClusters(snap.Cores)
	return snap, nil
}

func buildClusters(cores []Core) []Cluster {
	clusters := make([]Cluster, 0, 4)
	sig := func(c Core) string {
		return strconv.FormatInt(c.Min, 10) + "/" + strconv.FormatInt(c.Max, 10) + "/" + c.Governor
	}
	index := map[string]int{}
	for _, c := range cores {
		key := sig(c)
		if i, ok := index[key]; ok {
			cl := &clusters[i]
			cl.Cores = append(cl.Cores, c.Index)
			if c.Cur > cl.Cur {
				cl.Cur = c.Cur
			}
			continue
		}
		index[key] = len(clusters)
		clusters = append(clusters, Cluster{
			Index:    len(clusters),
			Cores:    []int{c.Index},
			Min:      c.Min,
			Max:      c.Max,
			Cur:      c.Cur,
			Governor: c.Governor,
			Freqs:    c.Freqs,
		})
	}
	return clusters
}

func parseInt(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

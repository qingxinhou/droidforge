package cpu

import (
	"context"
	"testing"
)

func fakeExec(out string) Exec {
	return func(ctx context.Context, command string) (string, error) { return out, nil }
}

func TestReadClusters(t *testing.T) {
	out := `C:0:1800000:300000:2800000:schedutil:300000 500000 1800000 2800000
C:1:1800000:300000:2800000:schedutil:300000 500000 1800000 2800000
C:2:2200000:300000:2800000:schedutil:300000 500000 1800000 2800000
C:7:2800000:700000:3200000:performance:700000 1200000 2800000 3200000`
	snap, err := Read(context.Background(), fakeExec(out))
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(snap.Cores) != 4 {
		t.Fatalf("核心数错误: %d", len(snap.Cores))
	}
	if snap.Cores[0].Index != 0 || snap.Cores[3].Index != 7 {
		t.Fatalf("核心顺序错误: %+v", snap.Cores)
	}
	if snap.Cores[0].Cur != 1800000 || snap.Cores[0].Max != 2800000 {
		t.Fatalf("频率解析错误: %+v", snap.Cores[0])
	}
	if len(snap.Cores[0].Freqs) != 4 || snap.Cores[0].Freqs[0] != 300000 {
		t.Fatalf("可用频率解析错误: %+v", snap.Cores[0].Freqs)
	}
	if len(snap.Clusters) != 2 {
		t.Fatalf("簇数量错误: %d", len(snap.Clusters))
	}
	if len(snap.Clusters[0].Cores) != 3 {
		t.Fatalf("第一个簇应含 3 个核心: %+v", snap.Clusters[0])
	}
	if snap.Clusters[1].Governor != "performance" {
		t.Fatalf("调频策略解析错误: %+v", snap.Clusters[1])
	}
}

func TestReadEmpty(t *testing.T) {
	snap, err := Read(context.Background(), fakeExec(""))
	if err != nil {
		t.Fatalf("空输出不应报错: %v", err)
	}
	if len(snap.Cores) != 0 || len(snap.Clusters) != 0 {
		t.Fatalf("空输出应无核心: %+v", snap)
	}
}

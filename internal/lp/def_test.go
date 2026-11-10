package lp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefinition(t *testing.T) {
	dir := t.TempDir()
	content := `# 动态分区定义
super_partition_groups=qti_dynamic_partitions
qti_dynamic_partitions_partition_list=system vendor product odm
qti_dynamic_partitions_size=9126805504
`
	if err := os.WriteFile(filepath.Join(dir, "super_def.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	def, err := LoadDefinition(dir)
	if err != nil {
		t.Fatal(err)
	}
	if def == nil {
		t.Fatal("应解析出定义")
	}
	if def.Source != "super_def.txt" {
		t.Fatalf("来源文件名错误：%q", def.Source)
	}
	if got := def.GroupOf("vendor"); got != "qti_dynamic_partitions" {
		t.Fatalf("vendor 分组错误：%q", got)
	}
	if got := def.MaxSizeOf("qti_dynamic_partitions"); got != 9126805504 {
		t.Fatalf("组容量错误：%d", got)
	}

	parts := ApplyDefinition([]BuildPartition{
		{Name: "system_a"},
		{Name: "vendor"},
		{Name: "unknown"},
	}, def)
	if parts[0].Group != "qti_dynamic_partitions" {
		t.Fatalf("带槽位后缀的分区也应匹配到组，实际 %q", parts[0].Group)
	}
	if parts[2].Group != "" {
		t.Fatalf("未声明的分区不应被强行分组，实际 %q", parts[2].Group)
	}
}

func TestLoadDefinitionMissing(t *testing.T) {
	def, err := LoadDefinition(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if def != nil {
		t.Fatal("没有定义文件时应返回 nil")
	}
}

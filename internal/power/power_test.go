package power

import (
	"context"
	"testing"
)

func fakeExec(out string) Exec {
	return func(ctx context.Context, command string) (string, error) { return out, nil }
}

func TestReadBattery(t *testing.T) {
	out := `Current Battery Service state:
  AC powered: false
  USB powered: true
  Wireless powered: false
  Max charging current: 3000000
  Max charging voltage: 5000000
  Charge counter: 4321000
  status: 2
  health: 2
  present: true
  level: 82
  scale: 100
  voltage: 4301
  temperature: 312
  technology: Li-ion
##CUR
-1250000
##VOLT
4301000
##CAP
82`
	info, err := Read(context.Background(), fakeExec(out))
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if info.Level != 82 || info.Scale != 100 {
		t.Fatalf("电量解析错误: %+v", info)
	}
	if !info.USBPowered || info.ACPowered || !info.Present {
		t.Fatalf("供电状态解析错误: %+v", info)
	}
	if info.StatusText != "充电中" || info.HealthText != "良好" {
		t.Fatalf("状态文本错误: %+v", info)
	}
	if info.VoltageV < 4.3 || info.VoltageV > 4.31 {
		t.Fatalf("电压换算错误: %v", info.VoltageV)
	}
	if info.TemperatureC < 31.1 || info.TemperatureC > 31.3 {
		t.Fatalf("温度换算错误: %v", info.TemperatureC)
	}
	if info.CurrentMA != -1250 {
		t.Fatalf("电流换算错误: %v", info.CurrentMA)
	}
	if info.PowerW < 5.3 || info.PowerW > 5.4 {
		t.Fatalf("功率换算错误: %v", info.PowerW)
	}
	if info.ChargeCounterMAh != 4321 {
		t.Fatalf("电荷计数换算错误: %v", info.ChargeCounterMAh)
	}
}

func TestMockValidation(t *testing.T) {
	exec := func(ctx context.Context, command string) (string, error) { return "", nil }
	if err := ApplyMock(context.Background(), exec, Mock{Level: 120}); err == nil {
		t.Fatalf("电量越界应报错")
	}
	if err := ApplyMock(context.Background(), exec, Mock{}); err == nil {
		t.Fatalf("空参数应报错")
	}
	if err := ApplyMock(context.Background(), exec, Mock{Level: 50}); err != nil {
		t.Fatalf("正常参数应通过: %v", err)
	}
}

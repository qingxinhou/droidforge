package edl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// 控制类报文（Hello / Done / 数据请求）的等待上限。
const controlIdle = 15 * time.Second

// 数据传输阶段允许的「静默」上限：单块最多 1 MB，慢速 USB 2.0 也就几秒。
const dataIdle = 30 * time.Second

// readFull 读满 buf，但**空闲超过 idle 就报错**。
//
// 不能直接用 io.ReadFull：WinUSB 的读超时按约定返回 (0, nil)，
// 而 io.ReadFull 把「0 字节 + nil」当成正常情况继续等，
// 设备一旦没响应就会永久卡在这里，界面既没有进度也没有错误。
func readFull(t Transport, buf []byte, idle time.Duration, what string) error {
	got := 0
	last := time.Now()
	for got < len(buf) {
		n, err := t.Read(buf[got:])
		if n > 0 {
			got += n
			last = time.Now()
			continue
		}
		if err != nil {
			return fmt.Errorf("%s：%w", what, err)
		}
		if time.Since(last) > idle {
			return fmt.Errorf("%s超时：%s 内没有收到任何数据", what, idle)
		}
	}
	return nil
}

// copyN 从 t 拷贝 n 字节到 w，带空闲超时与上下文检查。
//
// io.CopyN 同样是死等，读分区时若设备中途停发，界面会一直停在某个百分比上。
func copyN(ctx context.Context, w io.Writer, t Transport, n int64, idle time.Duration, what string) (int64, error) {
	buf := make([]byte, 256<<10)
	var done int64
	last := time.Now()
	for done < n {
		select {
		case <-ctx.Done():
			return done, ctx.Err()
		default:
		}
		want := int64(len(buf))
		if n-done < want {
			want = n - done
		}
		got, err := t.Read(buf[:want])
		if got > 0 {
			last = time.Now()
			if _, werr := w.Write(buf[:got]); werr != nil {
				return done, werr
			}
			done += int64(got)
			continue
		}
		if err != nil {
			return done, fmt.Errorf("%s：%w", what, err)
		}
		if time.Since(last) > idle {
			return done, fmt.Errorf("%s超时：已传 %d/%d 字节，%s 内没有新数据", what, done, n, idle)
		}
	}
	return done, nil
}

// errShortImage 表示本地镜像比声明的扇区数短。
var errShortImage = errors.New("本地镜像长度不足")

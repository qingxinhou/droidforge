//go:build icons

// 图标生成器：go run -tags icons ./tools/icongen
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

const (
	size = 1024
	ss   = 4
)

func main() {
	w := size * ss
	img := image.NewRGBA(image.Rect(0, 0, w, w))

	bg := color.RGBA{0x0B, 0x0F, 0x16, 0xFF}
	border := color.RGBA{0x1B, 0x26, 0x35, 0xFF}
	accent := color.RGBA{0x3B, 0x9E, 0xFF, 0xFF}
	cyan := color.RGBA{0x22, 0xD3, 0xEE, 0xFF}

	s := float64(ss)

	// 背景圆角矩形
	fillRoundRect(img, 0, 0, 1024*s, 1024*s, 210*s, bg)
	// 内描边
	strokeRoundRect(img, 8*s, 8*s, 1016*s, 1016*s, 204*s, 6*s, border)

	// 芯片引脚
	pinW := 24 * s
	for _, p := range []float64{368, 512, 656} {
		x := p * s
		drawLine(img, x, 232*s, x, 296*s, pinW, accent) // top
		drawLine(img, x, 728*s, x, 792*s, pinW, accent) // bottom
		y := p * s
		drawLine(img, 232*s, y, 296*s, y, pinW, accent) // left
		drawLine(img, 728*s, y, 792*s, y, pinW, accent) // right
	}

	// 芯片本体
	strokeRoundRect(img, 296*s, 296*s, 728*s, 728*s, 60*s, 26*s, accent)

	// 闪电
	bolt := [][2]float64{
		{560, 372}, {448, 540}, {516, 540}, {470, 660},
		{592, 486}, {522, 486}, {566, 372},
	}
	fillPolygon(img, bolt, s, cyan)

	out := downsample(img, ss)
	writePNG(out, "build/appicon.png")
	writePNG(resize(out, 256), "build/windows/icon.png")
	writeICO(resize(out, 256), "build/windows/icon.ico")
}

func fillRoundRect(img *image.RGBA, x0, y0, x1, y1, r float64, c color.RGBA) {
	for y := int(y0); y < int(y1); y++ {
		for x := int(x0); x < int(x1); x++ {
			if insideRound(x, y, x0, y0, x1, y1, r) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func strokeRoundRect(img *image.RGBA, x0, y0, x1, y1, r, w float64, c color.RGBA) {
	for y := int(y0) - 2; y < int(y1)+2; y++ {
		for x := int(x0) - 2; x < int(x1)+2; x++ {
			if insideRound(x, y, x0, y0, x1, y1, r) &&
				!insideRound(x, y, x0+w, y0+w, x1-w, y1-w, math.Max(0, r-w)) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func insideRound(x, y int, x0, y0, x1, y1, r float64) bool {
	fx, fy := float64(x)+0.5, float64(y)+0.5
	if fx < x0 || fx > x1 || fy < y0 || fy > y1 {
		return false
	}
	cx := math.Min(math.Max(fx, x0+r), x1-r)
	cy := math.Min(math.Max(fy, y0+r), y1-r)
	dx, dy := fx-cx, fy-cy
	return dx*dx+dy*dy <= r*r
}

func drawLine(img *image.RGBA, x0, y0, x1, y1, w float64, c color.RGBA) {
	minX := int(math.Min(x0, x1) - w)
	maxX := int(math.Max(x0, x1) + w)
	minY := int(math.Min(y0, y1) - w)
	maxY := int(math.Max(y0, y1) + w)
	r := w / 2
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if distToSeg(float64(x)+0.5, float64(y)+0.5, x0, y0, x1, y1) <= r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func distToSeg(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(px-x0, py-y0)
	}
	t := ((px-x0)*dx + (py-y0)*dy) / l2
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(x0+t*dx), py-(y0+t*dy))
}

func fillPolygon(img *image.RGBA, pts [][2]float64, s float64, c color.RGBA) {
	minX, minY := 1e9, 1e9
	maxX, maxY := -1e9, -1e9
	for _, p := range pts {
		x, y := p[0]*s, p[1]*s
		minX = math.Min(minX, x)
		maxX = math.Max(maxX, x)
		minY = math.Min(minY, y)
		maxY = math.Max(maxY, y)
	}
	n := len(pts)
	for y := int(minY); y <= int(maxY); y++ {
		for x := int(minX); x <= int(maxX); x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			inside := false
			for i, j := 0, n-1; i < n; j, i = i, i+1 {
				xi, yi := pts[i][0]*s, pts[i][1]*s
				xj, yj := pts[j][0]*s, pts[j][1]*s
				if (yi > fy) != (yj > fy) && fx < (xj-xi)*(fy-yi)/(yj-yi)+xi {
					inside = !inside
				}
			}
			if inside {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func downsample(src *image.RGBA, factor int) *image.RGBA {
	w := src.Bounds().Dx() / factor
	h := src.Bounds().Dy() / factor
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b, a uint32
			for dy := 0; dy < factor; dy++ {
				for dx := 0; dx < factor; dx++ {
					c := src.RGBAAt(x*factor+dx, y*factor+dy)
					r += uint32(c.R)
					g += uint32(c.G)
					b += uint32(c.B)
					a += uint32(c.A)
				}
			}
			n := uint32(factor * factor)
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), uint8(a / n)})
		}
	}
	return dst
}

func resize(src *image.RGBA, n int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			sx := x * sw / n
			sy := y * sh / n
			dst.SetRGBA(x, y, src.RGBAAt(sx, sy))
		}
	}
	return dst
}

func writePNG(img *image.RGBA, path string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}

// writeICO 生成单尺寸（256x256）PNG 编码的 ICO。
func writeICO(img *image.RGBA, path string) {
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		panic(err)
	}
	data := pngBuf.Bytes()

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(buf, binary.LittleEndian, uint16(1)) // type icon
	binary.Write(buf, binary.LittleEndian, uint16(1)) // count

	buf.WriteByte(0)                                  // width 256
	buf.WriteByte(0)                                  // height 256
	buf.WriteByte(0)                                  // color count
	buf.WriteByte(0)                                  // reserved
	binary.Write(buf, binary.LittleEndian, uint16(1)) // planes
	binary.Write(buf, binary.LittleEndian, uint16(32))
	binary.Write(buf, binary.LittleEndian, uint32(len(data)))
	binary.Write(buf, binary.LittleEndian, uint32(22))
	buf.Write(data)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		panic(err)
	}
}

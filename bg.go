package main

import (
	"math"
)

// 홈 배경 (움직이는 배경 5종). 미리보기 페이지(.workflow/animation-demo.html)와 같은 식을 Go 로 옮긴 것.
// 칸마다 (글자, 글자색, 배경색) 을 돌려주고, 색 표현은 grid.go 의 colSeq 가 터미널에 맞게 바꿈.
// H 는 omp.sh 의 정적 렌더링 방식(림에서 안쪽으로 점 밀도가 지수로 줄어듦)을 점 격자에 맞춰 옮긴 것.

type bgKind struct {
	ID, Name string
}

var bgKinds = []bgKind{
	{"A", "오로라"},
	{"B", "별빛"},
	{"C", "디지털 비"},
	{"E", "레트로 격자"},
	{"H", "디더링 행성"},
}

func validBg(id string) bool {
	for _, k := range bgKinds {
		if k.ID == id {
			return true
		}
	}
	return false
}

var (
	vFaint = vec{118, 118, 118}
	vBlack = vec{9, 9, 11}
)

// bgFill 은 격자 전체를 배경으로 채움
func bgFill(g *grid, kind string, t float64) {
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			r, fg, bg := bgCell(kind, x, y, g.w, g.h, t)
			g.c[y*g.w+x] = cell{r: r, w: 1, st: style{fg: colSeq(false, fg), bg: colSeq(true, bg)}}
		}
	}
}

func bgCell(kind string, x, y, w, h int, t float64) (rune, vec, vec) {
	fx, fy, fw, fh := float64(x), float64(y), float64(w), float64(h)
	switch kind {
	case "A":
		v := (math.Sin(fx*0.09+t*0.7) + math.Sin(fy*0.30-t*0.5) + math.Sin(fx*0.05+fy*0.18+t*0.4) +
			math.Sin(math.Sqrt(math.Pow((fx-fw/2)*0.5, 2)+math.Pow(fy-fh/2, 2)*1.6)*0.38-t*0.9)) / 4
		n := (v + 1) / 2
		c := gradV([]vec{{8, 10, 30}, {10, 44, 100}, {18, 128, 150}, {96, 64, 170}, {24, 22, 70}}, n)
		ch := ' '
		if n > 0.72 {
			ch = '░'
		}
		return ch, c.mul(1.25), c.mul(0.8)
	case "B":
		base := mixV(vec{6, 8, 22}, vec{16, 10, 40}, fy/fh)
		ch, fg := ' ', vFaint
		if hs := hash(fx, fy, 0); hs < 0.045 {
			tw := 0.5 + 0.5*math.Sin(t*(1.2+hs*40)+hs*90)
			switch {
			case tw > 0.85:
				ch = '+'
			case tw > 0.55:
				ch = '*'
			default:
				ch = '.'
			}
			fg = vec{200, 215, 255}.mul(0.35 + 0.65*tw)
		}
		if ph := frac(t / 5.5); ph < 0.35 { // 가끔 지나가는 유성
			p := ph / 0.35
			hx, hy := -6+p*(fw+12), -2+p*(fh*0.9)
			along := (hx - fx) / 2
			if along > 0 && along < 9 && math.Abs((hy-fy)-along*0.55) < 0.7 {
				ch, fg = '-', vec{255, 255, 255}.mul(1-along/9)
			}
			if math.Hypot((fx-hx)/2, fy-hy) < 0.9 {
				ch, fg = '*', vec{255, 255, 255}
			}
		}
		return ch, fg, base
	case "C":
		sp := 5 + hash(fx, 1, 0)*9
		off := hash(fx, 2, 0) * 100
		trail := 8 + math.Floor(hash(fx, 3, 0)*10)
		head := math.Mod(t*sp+off, fh+trail)
		d := head - fy
		ch, fg := ' ', vFaint
		if d >= 0 && d < trail {
			k := 1 - d/trail
			const glyphs = "0123456789ABCDEF:<>=+*"
			ch = rune(glyphs[int(hash(fx, fy, math.Floor(t*7+d))*float64(len(glyphs)))%len(glyphs)])
			if d < 1 {
				fg = vec{190, 255, 240}
			} else {
				fg = vec{30, 200, 120}.mul(0.15 + 0.55*k)
			}
		}
		return ch, fg, vec{2, 10, 8}
	case "E":
		yh := math.Floor(fh * 0.52)
		if fy < yh {
			k := fy / yh
			c := gradV([]vec{{14, 4, 40}, {70, 10, 90}, {160, 30, 120}, {250, 90, 110}}, k)
			dx, dy := (fx-fw/2)/2, fy-(yh-6)
			if math.Hypot(dx, dy) < 6.2 {
				stripe := (fy - (yh - 12)) / 12
				gap := stripe > 0.45 && math.Sin((fy+t*1.5)*2.6) > (1-stripe)*1.6-0.4
				if !gap {
					c = gradV([]vec{{255, 230, 90}, {255, 120, 120}, {220, 40, 170}}, stripe+0.1)
				}
			}
			return ' ', vFaint, c
		}
		p := (fy - yh + 0.5) / (fh - yh)
		z := 1 / p
		hl := frac(z*1.1 - t*1.0)
		vl := frac((fx-fw/2)/p/7 + 0.5)
		glow := clampF(0.25+p*0.95, 0, 1)
		bg := vec{10, 4, 24}
		switch {
		case hl < 0.16:
			return '-', vec{255, 60, 200}.mul(glow), bg
		case vl < 0.09 || vl > 0.95:
			return '|', vec{70, 200, 255}.mul(glow), bg
		}
		return ' ', vFaint, bg
	case "H":
		g := func(v *vec, k float64) vec {
			n := (hash(fx, k, 21) - 0.5) * 0.09 * 255
			if v == nil {
				return vBlack.add(n * 0.4)
			}
			return v.add(n)
		}
		return '▄', g(planetPix(x, 2*y+1, w, h, t), float64(2*y+1)+0.5), g(planetPix(x, 2*y, w, h, t), float64(2*y))
	}
	return ' ', vFaint, vBlack
}

// ---- H: 행성 ----
// 화면 밖 오른쪽 아래의 아주 큰 원. 림(테두리)에서 안쪽으로 들어간 깊이 l 에 따라 점이 찍힐 확률이 지수로 줄어듦.
// 점 한 개 = 반블록 한 칸의 위/아래 반쪽이라 가로 1칸 × 세로 0.5칸이고, 글꼴 비율상 거의 정사각형임.

type planetGeo struct{ cx, cy, r float64 }

func planetFor(w, h int) planetGeo {
	fw, fh := float64(w), float64(h)*2
	ax, ay := fw*0.27, fh
	bx, by := fw, fh*0.27
	r := fw * 0.95
	mx, my := (ax+bx)/2, (ay+by)/2
	vx, vy := bx-ax, by-ay
	c := math.Hypot(vx, vy)
	hh := math.Sqrt(math.Max(0, r*r-c*c/4))
	nx, ny := vy/c, -vx/c
	sg := 1.0
	if ny <= 0 {
		sg = -1
	}
	return planetGeo{cx: mx + sg*nx*hh, cy: my + sg*ny*hh, r: r}
}

var (
	cRim  = vec{140, 160, 255}
	cNear = vec{246, 92, 182}
	cFar  = vec{150, 34, 112}
)

// planetPix 는 점 하나의 색. nil 이면 배경(검정)
func planetPix(px, py, w, h int, t float64) *vec {
	pg := planetFor(w, h)
	x, y := float64(px), float64(py)
	dx, dy := x-pg.cx, y-pg.cy
	dist := math.Hypot(dx, dy)
	l := pg.r - dist
	ang := math.Atan2(dy, dx)
	gx, gy := x-float64(w)/2, y-float64(h)
	vig := math.Max(0.2, 1-math.Pow(math.Hypot(gx, gy)/(float64(w)*0.85), 3))
	ret := func(v vec) *vec { return &v }
	if l >= 0 {
		band := 1 + 0.16*math.Sin(ang*13-t*1.1) // 림을 따라 밝기 띠가 흐름
		if l < 1.3 {
			return ret(cRim.mul(vig * band))
		}
		tt := math.Exp(-l / 0.7)
		aa := math.Exp(-l/6) * math.Min(1, l/0.6) * 0.85
		oo := math.Exp(-l/30) * math.Min(1, l/1.5) * 0.7
		cc := math.Min(1, tt+aa+oo)
		seed := 0.0
		if hash(x, y, 3) < 0.14 { // 일부 점만 가끔 다시 뽑힘 (자글거림)
			seed = math.Floor(t * 3)
		}
		r := hash(x, y, 9+seed)
		switch {
		case r < tt:
			return ret(cRim.mul(vig * band))
		case r < tt+aa:
			return ret(cNear.mul(vig * band * (0.8 + 0.2*hash(x, y, 4))))
		case r < cc:
			return ret(cFar.mul(vig * (0.75 + 0.25*hash(x, y, 5))))
		}
		return nil
	}
	o := -l
	s2 := math.Floor(ang*pg.r*0.5 + t*2.4) // 호를 따라 천천히 흐르는 별가루
	o2 := math.Floor(o)
	pd := 0.7*math.Exp(-o/2.4) + 0.16*math.Exp(-o/10) + 0.02*math.Exp(-o/34)
	hs := hash(s2, o2, 5)
	if hs < pd {
		tw := 0.6 + 0.4*math.Sin(t*2.6+hs*200)
		return ret(vec{250, 250, 252}.mul((0.3 + 0.7*hash(s2, o2, 6)) * tw * vig))
	}
	if o < 2.2 {
		return ret(vec{100, 120, 255}.mul((1 - o/2.2) * 0.45 * vig))
	}
	return nil
}

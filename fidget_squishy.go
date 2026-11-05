package main

import (
	"fmt"
	"math"
	"math/rand"
	"unicode"
)

// 말랑이: 두 축 스프링(찌그러짐)과 가장자리 물결(젤리)로 움직이는 덩어리. 단위는 프레임(fidgetFrame) 하나.

const (
	squishyStiff      = 0.108 // 스프링 강성 (고유 진동수 약 0.33 rad/프레임, 주기 약 19프레임)
	squishyDamp       = 0.24  // 감쇠 계수 (감쇠비 약 0.3: 출렁 1~2번 뒤 가라앉음)
	squishyPress      = 0.50  // 꾹: 세로 충격량
	squishyPush       = 0.30  // 방향키 찌르기 충격량
	squishyPoke       = 0.12  // 아무 키 콕 충격량
	squishyShift      = 0.09  // 방향키가 몸통을 미는 충격량
	squishyMaxV       = 0.7   // 연타해도 속도는 이 값을 넘지 않음
	squishyMaxX       = 1.0   // 찌그러짐 한계 (로그 비율)
	squishyMaxStretch = 0.5   // 늘어남 한계 (로그 비율)
	squishyMaxShift   = 0.5   // 밀림 한계 (양방향 대칭)
	squishyVolume     = 0.85  // 부피 보존 정도 (1이면 완전 보존)
	squishyMinScale   = 0.4
	squishyMaxScale   = 1.4
	squishyFit        = 0.86 // 최대로 늘어났을 때 캔버스를 채우는 비율
	squishyAspect     = 1.25 // 평소 가로:세로 비율
	squishyPressed    = 0.8  // 한 축이 이 값보다 작아지면 얼굴이 > < o 로 바뀜
	squishyWobPress   = 0.09
	squishyWobPoke    = 0.04
	squishyWobMax     = 0.15
	squishyWobDecay   = 0.9
	squishyWobSpin    = 0.6
	squishyWobLobes   = 3
	squishyEps        = 0.012 // 이 아래면 가라앉은 것으로 보고 0 으로 스냅
	squishyShiftFrac  = 1.0   // 밀림 거리: 위치 * 세로 반지름
)

// squishyRand 는 콕 방향을 정하는 난수 (테스트에서 고정)
var squishyRand = rand.Float64

var squishyOmega = math.Sqrt(squishyStiff)

type squishySpring struct{ x, v float64 }

func (s *squishySpring) step(lo, hi float64) {
	s.v -= squishyStiff*s.x + squishyDamp*s.v
	s.x += s.v
	if s.x <= lo {
		s.x, s.v = lo, math.Max(0, s.v)
	} else if s.x >= hi {
		s.x, s.v = hi, math.Min(0, s.v)
	}
}

func (s *squishySpring) kick(dv float64) {
	s.v = math.Max(-squishyMaxV, math.Min(squishyMaxV, s.v+dv))
}

func (s *squishySpring) settled() bool {
	e := s.v / squishyOmega
	return s.x*s.x+e*e < squishyEps*squishyEps
}

type squishyMode struct {
	vert, horz   squishySpring
	offX, offY   squishySpring
	wob, wobAng  float64
	presses, hit int
}

func newSquishyMode() fidgetMode { return &squishyMode{} }

func (m *squishyMode) Title() string { return "말랑이" }

func (m *squishyMode) springs() [4]*squishySpring {
	return [4]*squishySpring{&m.vert, &m.horz, &m.offX, &m.offY}
}

func (m *squishyMode) wobble(amount float64) {
	m.wob = math.Min(squishyWobMax, m.wob+amount)
}

func (m *squishyMode) Key(k string) bool {
	switch k {
	case " ", "space", "enter":
		m.presses++
		m.vert.kick(-squishyPress)
		m.wobble(squishyWobPress)
	case "up":
		m.hit++
		m.vert.kick(-squishyPush)
		m.offY.kick(squishyShift)
		m.wobble(squishyWobPress)
	case "down":
		m.hit++
		m.vert.kick(-squishyPush)
		m.offY.kick(-squishyShift)
		m.wobble(squishyWobPress)
	case "left":
		m.hit++
		m.horz.kick(-squishyPush)
		m.offX.kick(squishyShift)
		m.wobble(squishyWobPress)
	case "right":
		m.hit++
		m.horz.kick(-squishyPush)
		m.offX.kick(-squishyShift)
		m.wobble(squishyWobPress)
	default:
		r := []rune(k)
		if len(r) != 1 || !(unicode.IsLetter(r[0]) || unicode.IsDigit(r[0])) {
			return false
		}
		m.hit++
		axis := &m.vert
		if squishyRand() >= 0.5 {
			axis = &m.horz
		}
		axis.kick(-squishyPoke)
		m.wobble(squishyWobPoke)
	}
	return true
}

func (m *squishyMode) Step() bool {
	m.vert.step(-squishyMaxX, squishyMaxStretch)
	m.horz.step(-squishyMaxX, squishyMaxStretch)
	m.offX.step(-squishyMaxShift, squishyMaxShift)
	m.offY.step(-squishyMaxShift, squishyMaxShift)
	m.wob *= squishyWobDecay
	m.wobAng += squishyWobSpin
	if m.wob > squishyEps/2 {
		return true
	}
	for _, s := range m.springs() {
		if !s.settled() {
			return true
		}
	}
	*m = squishyMode{presses: m.presses, hit: m.hit}
	return false
}

func (m *squishyMode) Status() string {
	if m.presses == 0 && m.hit == 0 {
		return "말랑이를 눌러 보세요"
	}
	return fmt.Sprintf("꾹 %d번 · 콕 %d번", m.presses, m.hit)
}

func (m *squishyMode) Keys() [][2]string {
	return [][2]string{{"Space", "꾹"}, {"←→↑↓", "찌르기"}}
}

// scales 는 현재 가로·세로 배율. 한 축이 줄면 다른 축이 늘어나도록 부피를 대략 보존함
func (m *squishyMode) scales() (sx, sy float64) {
	k := squishyVolume * (m.horz.x + m.vert.x) / 2
	clamp := func(v float64) float64 { return math.Max(squishyMinScale, math.Min(squishyMaxScale, v)) }
	return clamp(math.Exp(m.horz.x - k)), clamp(math.Exp(m.vert.x - k))
}

func (m *squishyMode) Rows(cw, ch int) [][]seg {
	if cw < 1 || ch < 1 {
		return make([][]seg, max(ch, 0))
	}
	c := newCanvas(cw, ch)
	m.paint(c)
	return c.Rows(colorAccent)
}

func (m *squishyMode) paint(c *canvas) {
	w, h := float64(c.W()), float64(c.H())
	ry0 := h / 2 * squishyFit / squishyMaxScale
	rx0 := math.Min(ry0*squishyAspect, w/2*squishyFit/squishyMaxScale)
	sx, sy := m.scales()
	rx, ry := rx0*sx, ry0*sy
	cx := w/2 + m.offX.x*ry0*squishyShiftFrac
	cy := h/2 + m.offY.x*ry0*squishyShiftFrac
	m.fillBody(c, cx, cy, rx, ry)

	face := newCanvas(c.cw, c.ch)
	squishyFace(face, cx, cy, rx, ry, math.Min(rx0, ry0), math.Min(sx, sy) < squishyPressed)
	for i, on := range face.px {
		if on {
			c.px[i] = false
		}
	}
}

func (m *squishyMode) fillBody(c *canvas, cx, cy, rx, ry float64) {
	pad := 1 + squishyWobMax
	for y := max(0, int(cy-ry*pad)); y <= min(c.H()-1, int(cy+ry*pad)+1); y++ {
		for x := max(0, int(cx-rx*pad)); x <= min(c.W()-1, int(cx+rx*pad)+1); x++ {
			u, v := (float64(x)+0.5-cx)/rx, (float64(y)+0.5-cy)/ry
			edge := 1 + m.wob*math.Cos(squishyWobLobes*math.Atan2(v, u)-m.wobAng)
			if math.Hypot(u, v) <= edge {
				c.px[y*c.W()+x] = true
			}
		}
	}
}

// squishyFace 는 지울 점(눈·입)을 f 에 그림
func squishyFace(f *canvas, cx, cy, rx, ry, unit float64, pressed bool) {
	eyeY := int(math.Round(cy - ry*0.15))
	eyeL, eyeR := int(math.Round(cx-rx*0.4)), int(math.Round(cx+rx*0.4))
	mouthX, mouthY := int(math.Round(cx)), int(math.Round(cy+ry*0.38))
	if pressed {
		e := max(3, int(math.Round(unit*0.22)))
		for dx := 0; dx <= 1; dx++ {
			f.Line(eyeL-e+dx, eyeY-e, eyeL+e+dx, eyeY)
			f.Line(eyeL+e+dx, eyeY, eyeL-e+dx, eyeY+e)
			f.Line(eyeR+e-dx, eyeY-e, eyeR-e-dx, eyeY)
			f.Line(eyeR-e-dx, eyeY, eyeR+e-dx, eyeY+e)
		}
		f.FillEllipse(float64(mouthX), float64(mouthY), math.Max(1.5, unit*0.12), math.Max(2, unit*0.17))
		return
	}
	dot := math.Max(0.8, unit*0.11)
	f.FillCircle(float64(eyeL), float64(eyeY), dot)
	f.FillCircle(float64(eyeR), float64(eyeY), dot)
	half := max(2, int(math.Round(rx*0.2)))
	depth := math.Max(1, math.Round(ry*0.1))
	for i := -half; i <= half; i++ {
		t := float64(i) / float64(half)
		y := mouthY + int(math.Round(depth*(1-t*t)))
		f.Set(mouthX+i, y)
		f.Set(mouthX+i, y+1)
	}
}

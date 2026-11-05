package main

import (
	"fmt"
	"math"

	"github.com/charmbracelet/lipgloss"
)

// 피젯 스피너 모드. 날개 3개짜리 스피너를 점자 캔버스에 그리고 각속도로 돌림.

const (
	spinnerKick     = 0.30  // Space/Enter/↑ 한 번의 충격량 (rad/프레임)
	spinnerNudge    = 0.15  // ←/→ 한 번의 충격량
	spinnerMaxOmega = 1.0   // 속도 상한 (rad/프레임)
	spinnerDecay    = 0.985 // 프레임당 지수 감쇠
	spinnerFriction = 0.0005
	spinnerSnap     = 0.002 // 이 속도 미만이면 0 으로 멈춤
	spinnerStartAng = -math.Pi / 2
	spinnerSector   = 2 * math.Pi / 3
)

type spinnerMode struct {
	angle float64 // 화면 기준 시계 방향이 양수
	omega float64
	turns float64 // 방향 상관없이 누적한 바퀴 수
}

func newSpinnerMode() fidgetMode { return &spinnerMode{angle: spinnerStartAng} }

func (s *spinnerMode) Title() string { return "스피너" }

func (s *spinnerMode) Key(k string) bool {
	switch k {
	case " ", "space", "enter", "up":
		s.push(spinnerKick)
	case "right":
		s.push(spinnerNudge)
	case "left":
		s.push(-spinnerNudge)
	case "r":
		s.turns = 0
	default:
		return false
	}
	return true
}

func (s *spinnerMode) push(impulse float64) {
	s.omega = math.Max(-spinnerMaxOmega, math.Min(spinnerMaxOmega, s.omega+impulse))
}

func (s *spinnerMode) Step() bool {
	if s.omega == 0 {
		return false
	}
	s.angle = math.Mod(s.angle+s.omega, 2*math.Pi)
	s.turns += math.Abs(s.omega) / (2 * math.Pi)
	next := s.omega*spinnerDecay - math.Copysign(spinnerFriction, s.omega)
	if next*s.omega <= 0 || math.Abs(next) < spinnerSnap {
		next = 0
	}
	s.omega = next
	return s.omega != 0
}

func (s *spinnerMode) Status() string {
	text := fmt.Sprintf("%.1f바퀴 · %s", s.turns, spinnerLevel(s.omega))
	switch {
	case s.omega > 0:
		text += " · 시계"
	case s.omega < 0:
		text += " · 반시계"
	}
	return text
}

func (s *spinnerMode) Keys() [][2]string {
	return [][2]string{{"Space", "튕기기"}, {"←→", "방향"}, {"r", "초기화"}}
}

func spinnerLevel(omega float64) string {
	switch w := math.Abs(omega); {
	case w == 0:
		return "정지"
	case w < 0.1:
		return "천천히"
	case w < 0.35:
		return "보통"
	case w < 0.7:
		return "빠르게"
	default:
		return "쌩쌩"
	}
}

func spinnerBodyColor(omega float64) lipgloss.Color {
	switch w := math.Abs(omega); {
	case w == 0:
		return colorTextMuted
	case w < 0.35:
		return colorText
	case w < 0.7:
		return colorAccent
	default:
		return colorOk
	}
}

// spinnerGhosts 는 속도에 따라 겹쳐 그릴 잔상 개수
func spinnerGhosts(omega float64) int {
	switch w := math.Abs(omega); {
	case w >= 0.6:
		return 3
	case w >= 0.35:
		return 2
	case w >= 0.15:
		return 1
	}
	return 0
}

func (s *spinnerMode) Rows(cw, ch int) [][]seg {
	if ch <= 0 {
		return nil
	}
	rows := make([][]seg, ch)
	if cw <= 0 {
		return rows
	}
	all, body := newCanvas(cw, ch), newCanvas(cw, ch)
	for k := spinnerGhosts(s.omega); k >= 1; k-- {
		spinnerPaint(all, s.angle-s.omega*float64(k), true)
	}
	spinnerPaint(body, s.angle, false)
	for i, on := range body.px {
		if on {
			all.px[i] = true
		}
	}
	bodyCol := spinnerBodyColor(s.omega)
	for cy := range rows {
		rows[cy] = all.RowFunc(cy, func(cx int) lipgloss.Color {
			if body.cell(cx, cy) != " " {
				return bodyCol
			}
			return colorTextMuted
		})
	}
	return rows
}

// spinnerPaint 는 캔버스 가운데에 스피너를 그림. thin 이면 날개 테두리만 그림 (잔상용)
func spinnerPaint(c *canvas, angle float64, thin bool) {
	cx, cy := float64(c.W())/2, float64(c.H())/2
	reach := math.Max(1, float64(min(c.W(), c.H()))/2-1)
	armLen, wingR := 0.60*reach, 0.38*reach
	wingHole, armHalf := 0.20*reach, 0.16*reach
	hubR, hubHole := 0.26*reach, 0.10*reach
	x0, x1 := int(math.Floor(cx-reach-1)), int(math.Ceil(cx+reach+1))
	y0, y1 := int(math.Floor(cy-reach-1)), int(math.Ceil(cy+reach+1))
	for y := max(0, y0); y <= min(c.H()-1, y1); y++ {
		for x := max(0, x0); x <= min(c.W()-1, x1); x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			r := math.Hypot(dx, dy)
			if r > reach+1 {
				continue
			}
			th := math.Mod(math.Atan2(dy, dx)-angle+spinnerSector/2, spinnerSector)
			if th < 0 {
				th += spinnerSector
			}
			th -= spinnerSector / 2
			lx, ly := r*math.Cos(th), r*math.Sin(th)
			dw := math.Hypot(lx-armLen, ly)
			if thin {
				if math.Abs(dw-wingR) <= 0.7 {
					c.Set(x, y)
				}
				continue
			}
			wing := dw <= wingR && dw >= wingHole
			arm := lx >= 0 && lx <= armLen-wingHole && math.Abs(ly) <= armHalf
			if (wing || arm || r <= hubR) && r >= hubHole {
				c.Set(x, y)
			}
		}
	}
}

package main

import (
	"math"
	"time"
)

// 화면 전환 4종. 앱을 열 때와 홈으로 돌아올 때 약 0.7초 동안 옛 화면에서 새 화면으로 바뀜.
// a=옛 화면, b=새 화면 (둘 다 w×h 격자), u=진행도 0..1. 넓은 글자(한글)는 앞칸·뒷칸이 같은 쪽으로 정해짐.

const transDur = 700 * time.Millisecond

type transKind struct {
	ID   int // 설정 파일에 저장되는 번호. 0 은 끔
	Name string
}

var transKinds = []transKind{
	{1, "빛나는 와이프"},
	{2, "원형 열기"},
	{3, "디졸브"},
	{4, "밀어내기"},
}

func validTrans(id int) bool { return id >= 0 && id <= len(transKinds) }

// leadAB 는 (x,y) 칸이 넓은 글자의 뒷칸이면 앞칸 x 를, 아니면 x 를 돌려줌 (두 화면 어느 쪽이든 뒷칸이면 앞칸에 묶음)
func leadAB(a, b *grid, x, y int) int {
	if x > 0 && (a.at(x, y).w == 0 || b.at(x, y).w == 0) {
		return x - 1
	}
	return x
}

func fxCell(r rune, fg, bg vec) cell {
	return cell{r: r, w: 1, st: style{fg: colSeq(false, fg), bg: colSeq(true, bg)}}
}

func blend(kind int, a, b *grid, u float64, reverse bool) *grid {
	g := newGrid(a.w, a.h)
	W, H := float64(a.w), float64(a.h)
	pickA := func(x, y int) cell { return *a.at(x, y) }
	pickB := func(x, y int) cell { return *b.at(x, y) }
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			ex := float64(leadAB(a, b, x, y))
			var out cell
			switch kind {
			case 1: // 빛나는 와이프: 대각선 빛줄기가 훑고 지나감
				s := ex/W*0.72 + float64(y)/H*0.28
				d := s - (easeIO(u)*1.25 - 0.12)
				switch {
				case d < -0.05:
					out = pickB(x, y)
				case d < 0.07:
					k := 1 - math.Abs(d)/0.07
					ch := '▒'
					if k > 0.66 {
						ch = '█'
					} else if k > 0.33 {
						ch = '▓'
					}
					out = fxCell(ch, gradV([]vec{{60, 120, 255}, {97, 214, 214}, {255, 255, 255}}, k), gradV([]vec{{10, 20, 60}, {30, 70, 110}, {120, 200, 220}}, k))
				default:
					out = pickA(x, y)
				}
			case 2: // 원형 열기: 중앙에서 빛나는 고리가 퍼짐
				r := math.Hypot((ex-W/2)/2, float64(y)-H/2)
				maxR := math.Hypot(W/4, H/2) + 2
				R := easeIO(u)*(maxR+3) - 1
				switch {
				case r < R-1.6:
					out = pickB(x, y)
				case r < R+0.2:
					k := 1 - (R-r)/1.8
					ch := '▒'
					if k > 0.6 {
						ch = '▓'
					}
					out = fxCell(ch, gradV([]vec{{97, 214, 214}, {255, 255, 255}}, k), gradV([]vec{{20, 60, 90}, {140, 220, 240}}, k))
				default:
					out = pickA(x, y)
				}
			case 3: // 디졸브: 칸마다 ░▒▓█▓▒░ 로 짙어졌다가 옅어지며 새 화면으로 바뀜. 전환 전체로도 중간(u=0.5)에 가장 밝음
				n := hash(math.Floor(ex/2), float64(y), 7)
				th := u*1.5 - 0.1
				switch {
				case n < th-0.36:
					out = pickB(x, y)
				case n < th+0.02:
					p := clampF((th+0.02-n)/0.38, 0, 1) // 칸 하나의 진행: 0 → 1
					tri := 1 - math.Abs(2*p-1)          // 0 → 1 → 0 (가운데가 가장 짙음)
					env := 0.25 + 0.75*math.Sin(math.Pi*u)
					k := tri * env
					ch := '░'
					switch {
					case k > 0.7:
						ch = '█'
					case k > 0.45:
						ch = '▓'
					case k > 0.2:
						ch = '▒'
					}
					under := pickA(x, y) // 앞쪽 절반은 옛 화면 바탕, 뒤쪽 절반은 새 화면 바탕 위에서 옅어짐
					if p >= 0.5 {
						under = pickB(x, y)
					}
					out = cell{r: ch, w: 1, st: style{fg: colSeq(false, gradV([]vec{{60, 120, 200}, {97, 214, 214}, {235, 250, 255}}, k)), bg: under.st.bg}}
				default:
					out = pickA(x, y)
				}
			default: // 4 밀어내기: 새 화면이 옆에서 밀고 들어옴
				off := int(math.Round(easeIO(u) * W))
				if !reverse {
					xa := x + off
					switch {
					case off > 0 && off < a.w && x == a.w-off:
						out = fxCell('|', vec{200, 240, 255}, vec{30, 70, 110})
					case xa < a.w:
						out = pickA(xa, y)
					default:
						out = pickB(xa-a.w, y)
					}
				} else {
					xa := x - off
					switch {
					case off > 0 && off < a.w && x == off:
						out = fxCell('|', vec{200, 240, 255}, vec{30, 70, 110})
					case xa >= 0:
						out = pickA(xa, y)
					default:
						out = pickB(xa+a.w, y)
					}
				}
			}
			g.c[y*g.w+x] = out
		}
	}
	g.sanitize()
	return g
}

// transDuration 은 전환 종류별 길이. 지금은 모두 같음 (디졸브를 1.5초로 늘려봤다가 원래 속도로 되돌림)
func transDuration(kind int) time.Duration { return transDur }

package main

import (
	"math"
	"math/rand"
	"strings"
	"testing"
)

func fixedSquishyRand(t *testing.T, v float64) {
	t.Helper()
	old := squishyRand
	squishyRand = func() float64 { return v }
	t.Cleanup(func() { squishyRand = old })
}

func newSquishy() *squishyMode { return newSquishyMode().(*squishyMode) }

// settleFrames 는 Step 이 false 를 돌려줄 때까지의 프레임 수 (limit 안에 안 멈추면 -1)
func settleFrames(m *squishyMode, limit int) int {
	for i := 1; i <= limit; i++ {
		if !m.Step() {
			return i
		}
	}
	return -1
}

func TestSquishyKeysChangeState(t *testing.T) {
	fixedSquishyRand(t, 0.1)
	for _, k := range []string{" ", "space", "enter"} {
		m := newSquishy()
		if !m.Key(k) || m.vert.v >= 0 || m.presses != 1 {
			t.Errorf("%q: 꾹이 세로 충격으로 반영되지 않음: %+v", k, m)
		}
	}
	cases := map[string]func(m *squishyMode) bool{
		"up":    func(m *squishyMode) bool { return m.vert.v < 0 && m.offY.v > 0 },
		"down":  func(m *squishyMode) bool { return m.vert.v < 0 && m.offY.v < 0 },
		"left":  func(m *squishyMode) bool { return m.horz.v < 0 && m.offX.v > 0 },
		"right": func(m *squishyMode) bool { return m.horz.v < 0 && m.offX.v < 0 },
		"a":     func(m *squishyMode) bool { return m.vert.v < 0 && m.horz.v == 0 },
		"7":     func(m *squishyMode) bool { return m.vert.v < 0 && m.horz.v == 0 },
		"한":     func(m *squishyMode) bool { return m.vert.v < 0 },
	}
	for k, ok := range cases {
		m := newSquishy()
		if !m.Key(k) || !ok(m) || m.hit != 1 || m.presses != 0 {
			t.Errorf("%q 키가 기대한 방향으로 반영되지 않음: %+v", k, m)
		}
	}
	m := newSquishy()
	for _, k := range []string{"", "tab", "f1", "backspace", "ctrl+c", "?", "ab"} {
		if m.Key(k) {
			t.Errorf("%q 는 무시해야 함", k)
		}
	}
	if *m != (squishyMode{}) {
		t.Errorf("무시한 키가 상태를 바꿨음: %+v", m)
	}
}

func TestSquishyPokeAxisFollowsRand(t *testing.T) {
	fixedSquishyRand(t, 0.9)
	m := newSquishy()
	m.Key("x")
	if m.horz.v >= 0 || m.vert.v != 0 {
		t.Errorf("난수 0.9 면 가로 콕이어야 함: %+v", m)
	}
}

func TestSquishyAxesTradeVolume(t *testing.T) {
	cases := map[string]func(sx, sy float64) bool{
		" ":    func(sx, sy float64) bool { return sy < 0.8 && sx > 1.1 },
		"left": func(sx, sy float64) bool { return sx < 0.9 && sy > 1.05 },
	}
	for k, ok := range cases {
		m := newSquishy()
		m.Key(k)
		hit := false
		for i := 0; i < 40; i++ {
			m.Step()
			sx, sy := m.scales()
			if a := sx * sy; a < 0.75 || a > 1.3 {
				t.Fatalf("%q: 프레임 %d 에서 부피가 %.2f 배로 변함", k, i, a)
			}
			hit = hit || ok(sx, sy)
		}
		if !hit {
			t.Errorf("%q: 한 축이 줄 때 다른 축이 늘어나는 순간이 없음", k)
		}
	}
}

func TestSquishyFreshModeIsAtRest(t *testing.T) {
	if newSquishy().Step() {
		t.Errorf("아무것도 안 눌렀는데 Step 이 true")
	}
}

func TestSquishySettlesAfterAnyMashing(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	keys := []string{" ", "enter", "up", "down", "left", "right", "a", "z", "5", "tab", "?"}
	for trial := 0; trial < 20; trial++ {
		m := newSquishy()
		for i := 0; i < 300; i++ {
			m.Key(keys[r.Intn(len(keys))])
			if r.Intn(3) == 0 {
				m.Step()
			}
			for _, s := range m.springs() {
				if s.v > squishyMaxV || s.v < -squishyMaxV || s.x < -squishyMaxX || s.x > squishyMaxStretch {
					t.Fatalf("연타로 상한을 넘음: %+v", s)
				}
			}
			if m.wob > squishyWobMax {
				t.Fatalf("물결이 상한을 넘음: %v", m.wob)
			}
		}
		if n := settleFrames(m, 120); n < 0 {
			t.Fatalf("연타 뒤 120프레임 안에 멈추지 않음: %+v", m)
		}
	}
}

func TestSquishySnapsToZeroWhenSettled(t *testing.T) {
	m := newSquishy()
	m.Key(" ")
	m.Key("left")
	m.Key("x")
	if settleFrames(m, 200) < 0 {
		t.Fatalf("멈추지 않음")
	}
	if *m != (squishyMode{presses: 1, hit: 2}) {
		t.Errorf("멈췄는데 위치·속도가 0 이 아님: %+v", m)
	}
	if m.Step() {
		t.Errorf("멈춘 뒤 Step 이 다시 true")
	}
}

func TestSquishySingleSqueezeTuning(t *testing.T) {
	m := newSquishy()
	m.Key(" ")
	frames, flips, lastVisible, prev := 0, 0, 0, 0
	minScale := 9.0
	for i := 1; i <= 200; i++ {
		live := m.Step()
		sx, sy := m.scales()
		minScale = min(minScale, sx, sy)
		sign := 0
		switch {
		case m.vert.x > 0.04:
			sign = 1
		case m.vert.x < -0.04:
			sign = -1
		}
		if sign != 0 {
			if prev != 0 && sign != prev {
				flips++
			}
			prev, lastVisible = sign, i
		}
		if !live {
			frames = i
			break
		}
	}
	if frames < 30 || frames > 45 {
		t.Errorf("가라앉는 데 %d프레임 (기대 30~45)", frames)
	}
	if flips < 1 || flips > 2 {
		t.Errorf("출렁임(부호 반전) %d번 (기대 1~2)", flips)
	}
	if lastVisible > 32 {
		t.Errorf("눈에 띄는 찌그러짐이 %d프레임까지 남음", lastVisible)
	}
	if minScale >= squishyPressed {
		t.Errorf("꾹 눌렀는데 충분히 납작해지지 않음: %.2f", minScale)
	}
}

func TestSquishyStackedSqueezeIsBoundedAndStillSettles(t *testing.T) {
	m := newSquishy()
	for i := 0; i < 10; i++ {
		m.Key(" ")
	}
	single := newSquishy()
	single.Key(" ")
	if m.vert.v >= single.vert.v {
		t.Errorf("연타해도 충격량이 쌓이지 않음")
	}
	settled := false
	for i := 0; i < 120 && !settled; i++ {
		settled = !m.Step()
		if sx, sy := m.scales(); sx > squishyMaxScale || sy < squishyMinScale {
			t.Fatalf("배율이 한계를 넘음: %.2f %.2f", sx, sy)
		}
	}
	if !settled {
		t.Errorf("연타 뒤에도 120프레임 안에는 멈춰야 함")
	}
}

func TestSquishyRowsGeometry(t *testing.T) {
	check := func(m *squishyMode, cw, ch int) {
		t.Helper()
		rows := m.Rows(cw, ch)
		if len(rows) != ch {
			t.Fatalf("%dx%d: %d줄 (기대 %d)", cw, ch, len(rows), ch)
		}
		for i, r := range rows {
			if w := segsWidth(r); w > cw {
				t.Fatalf("%dx%d: %d번째 줄 폭 %d 가 cw 를 넘음", cw, ch, i, w)
			}
		}
	}
	poses := []func(m *squishyMode){
		func(m *squishyMode) {},
		func(m *squishyMode) { m.Key(" "); m.Step(); m.Step(); m.Step() },
		func(m *squishyMode) {
			m.Key("left")
			m.Key("up")
			m.Step()
			m.Step()
			m.Step()
			m.Step()
			m.Step()
		},
		func(m *squishyMode) {
			for i := 0; i < 30; i++ {
				m.Key(" ")
				m.Key("right")
				m.Step()
			}
		},
	}
	for pi, pose := range poses {
		m := newSquishy()
		pose(m)
		for _, sz := range testSizes {
			check(m, sz[0]-10, min(14, max(3, sz[1]-9)))
		}
		for _, cw := range []int{1, 2, 3, 4, 7, 50, 200} {
			for _, ch := range []int{1, 2, 3, 5, 20} {
				check(m, cw, ch)
			}
		}
		if pi == 0 && !strings.ContainsAny(plainRows(m.Rows(40, 14)), "⣿⣷⣾⣶⣦⣴") {
			t.Errorf("쉬는 상태에서 몸통이 그려지지 않음")
		}
	}
	if rows := newSquishy().Rows(0, 0); len(rows) != 0 {
		t.Errorf("0 크기에서 줄이 나옴")
	}
}

func plainRows(rows [][]seg) string {
	var b strings.Builder
	for _, r := range rows {
		for _, s := range r {
			b.WriteString(s.t)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestSquishyRandomKeysAndSizesDoNotPanic(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	keys := []string{" ", "space", "enter", "up", "down", "left", "right", "a", "Q", "9", "tab", "esc", "", "ctrl+a", "한"}
	m := newSquishy()
	for i := 0; i < 3000; i++ {
		m.Key(keys[r.Intn(len(keys))])
		for j := r.Intn(4); j > 0; j-- {
			m.Step()
		}
		cw, ch := 1+r.Intn(200), 1+r.Intn(20)
		if rows := m.Rows(cw, ch); len(rows) != ch {
			t.Fatalf("%dx%d: %d줄", cw, ch, len(rows))
		}
		m.Status()
		m.Keys()
	}
}

func TestSquishyResizeKeepsState(t *testing.T) {
	m := newSquishy()
	m.Key(" ")
	m.Key("right")
	for i := 0; i < 4; i++ {
		m.Step()
	}
	before := *m
	for _, sz := range [][2]int{{46, 8}, {70, 14}, {5, 3}, {200, 20}, {46, 8}} {
		m.Rows(sz[0], sz[1])
		if *m != before {
			t.Fatalf("%v 로 그린 뒤 상태가 바뀜", sz)
		}
	}
}

func TestSquishyFaceChangesWhenSquashed(t *testing.T) {
	m := newSquishy()
	rest := plainRows(m.Rows(40, 14))
	m.Key(" ")
	for i := 0; i < 4; i++ {
		m.Step()
	}
	if plainRows(m.Rows(40, 14)) == rest {
		t.Errorf("눌렀는데 모양이 그대로임")
	}
	a, b := newCanvas(30, 10), newCanvas(30, 10)
	squishyFace(a, 30, 20, 20, 15, 15, false)
	squishyFace(b, 30, 20, 20, 15, 15, true)
	same := true
	for i := range a.px {
		same = same && a.px[i] == b.px[i]
	}
	if same {
		t.Errorf("눌린 얼굴(> < o)이 평소 얼굴과 같음")
	}
}

func TestSquishyStatusAndKeys(t *testing.T) {
	m := newSquishy()
	if m.Title() != "말랑이" {
		t.Errorf("탭 이름: %q", m.Title())
	}
	if m.Status() == "" {
		t.Errorf("처음 상태 문구가 비어 있음")
	}
	m.Key(" ")
	m.Key("enter")
	m.Key("up")
	if got := m.Status(); !strings.Contains(got, "꾹 2번") {
		t.Errorf("꾹 횟수가 안 보임: %q", got)
	}
	keys := m.Keys()
	if len(keys) < 2 {
		t.Fatalf("키 안내가 부족함: %v", keys)
	}
	for _, k := range keys {
		if k[0] == "" || k[1] == "" {
			t.Errorf("빈 키 안내: %v", k)
		}
	}
}

func TestSquishyInsideFidgetApp(t *testing.T) {
	a := newFidgetApp(&Env{}).(*fidgetApp)
	a.cur = 1
	if a.modes[1].Title() != "말랑이" {
		t.Fatalf("두 번째 탭이 말랑이가 아님")
	}
	if _, cmd := a.Update(key(" ")); cmd == nil {
		t.Errorf("스페이스 뒤 틱이 시작되지 않음")
	}
	tick := appTickMsg{Gen: a.gen, Tag: a.tag()}
	frames := 0
	for ; a.ticking && frames < 200; frames++ {
		a.Update(tick)
	}
	if a.ticking {
		t.Errorf("200프레임 안에 호스트 틱이 멈추지 않음")
	}
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		assertFrame(t, "말랑이", c, a.View(c))
	}
}

func squishyShiftPeak(key string, perFrame, frames int) (lo, hi float64) {
	m := newSquishy()
	for f := 0; f < frames; f++ {
		for i := 0; i < perFrame; i++ {
			m.Key(key)
		}
		m.Step()
		for _, s := range []*squishySpring{&m.offX, &m.offY} {
			lo, hi = min(lo, s.x), max(hi, s.x)
		}
	}
	return lo, hi
}

func TestSquishyShiftIsSymmetricPerDirection(t *testing.T) {
	pairs := [][2]string{{"left", "right"}, {"up", "down"}}
	for _, rate := range [][2]int{{1, 90}, {2, 60}, {5, 60}} {
		for _, p := range pairs {
			aLo, aHi := squishyShiftPeak(p[0], rate[0], rate[1])
			bLo, bHi := squishyShiftPeak(p[1], rate[0], rate[1])
			if math.Abs(aHi+bLo) > 1e-9 || math.Abs(aLo+bHi) > 1e-9 {
				t.Errorf("%v 연타 %v: %s [%.3f,%.3f] 와 %s [%.3f,%.3f] 가 대칭 아님", rate, p, p[0], aLo, aHi, p[1], bLo, bHi)
			}
			if math.Max(aHi, bHi) > squishyMaxShift+1e-9 || math.Min(aLo, bLo) < -squishyMaxShift-1e-9 {
				t.Errorf("밀림 한계 초과: %v %v", p, rate)
			}
		}
	}
}

func TestSquishyWallStopsOutwardVelocity(t *testing.T) {
	for _, key := range []string{"left", "right", "up", "down"} {
		m := newSquishy()
		wall := 0
		for f := 0; f < 90; f++ {
			m.Key(key)
			m.Key(key)
			m.Step()
			for _, s := range []*squishySpring{&m.offX, &m.offY} {
				if s.x >= squishyMaxShift && s.v > 0 || s.x <= -squishyMaxShift && s.v < 0 {
					t.Fatalf("%s: 벽에서 바깥 속도가 남음 %+v", key, *s)
				}
				if math.Abs(s.x) >= squishyMaxShift {
					wall++
				}
			}
		}
		if wall == 0 {
			t.Errorf("%s: 벽에 닿는 상황을 만들지 못함", key)
		}
	}
}

func squishyBodyExtent(m *squishyMode, cw, ch int) (left, right int) {
	c := newCanvas(cw, ch)
	m.paint(c)
	left, right = c.W(), -1
	for i, on := range c.px {
		if on {
			left, right = min(left, i%c.W()), max(right, i%c.W())
		}
	}
	return left, right
}

func TestSquishyMashedBodyIsMirrored(t *testing.T) {
	const cw, ch = 40, 14
	push := func(key string) *squishyMode {
		m := newSquishy()
		for f := 0; f < 60; f++ {
			m.Key(key)
			m.Key(key)
			m.Step()
		}
		return m
	}
	lL, rL := squishyBodyExtent(push("left"), cw, ch)
	lR, rR := squishyBodyExtent(push("right"), cw, ch)
	w := cw * 2
	if d := (lL - 0) - (w - 1 - rR); d < -1 || d > 1 {
		t.Errorf("좌우 연타 몸통 위치가 거울상이 아님: left=[%d,%d] right=[%d,%d]", lL, rL, lR, rR)
	}
}

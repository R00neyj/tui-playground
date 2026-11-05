package main

import (
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func spinnerOf(t *testing.T) *spinnerMode {
	t.Helper()
	s, ok := newSpinnerMode().(*spinnerMode)
	if !ok {
		t.Fatalf("newSpinnerMode 가 *spinnerMode 가 아님")
	}
	return s
}

// spinnerFramesToStop 은 Step 이 false 를 돌려줄 때까지의 프레임 수. limit 를 넘으면 -1
func spinnerFramesToStop(s *spinnerMode, limit int) int {
	for n := 1; n <= limit; n++ {
		if !s.Step() {
			return n
		}
	}
	return -1
}

func TestSpinnerKeysChangeState(t *testing.T) {
	s := spinnerOf(t)
	for _, k := range []string{" ", "space", "enter", "up"} {
		before := s.omega
		if !s.Key(k) || s.omega <= before {
			t.Errorf("%q: 시계 방향으로 빨라져야 함 (%v -> %v)", k, before, s.omega)
		}
	}
	s = spinnerOf(t)
	if !s.Key("left") || s.omega >= 0 {
		t.Errorf("left: 반시계 방향이어야 함: %v", s.omega)
	}
	s = spinnerOf(t)
	if !s.Key("right") || s.omega <= 0 {
		t.Errorf("right: 시계 방향이어야 함: %v", s.omega)
	}
	if s.Key("x") || s.Key("down") {
		t.Errorf("처리하지 않는 키는 false 여야 함")
	}
	s.turns = 5
	if !s.Key("r") || s.turns != 0 {
		t.Errorf("r: 회전수가 초기화돼야 함: %v", s.turns)
	}
}

func TestSpinnerMashSpeedsUpAndCaps(t *testing.T) {
	s := spinnerOf(t)
	prev := 0.0
	for i := 0; i < 3; i++ {
		s.Key(" ")
		if s.omega <= prev {
			t.Fatalf("연타해도 빨라지지 않음: %v -> %v", prev, s.omega)
		}
		prev = s.omega
	}
	for i := 0; i < 50; i++ {
		s.Key(" ")
		s.Key("left")
		s.Key("right")
	}
	if math.Abs(s.omega) > spinnerMaxOmega {
		t.Errorf("속도 상한을 넘음: %v", s.omega)
	}
	s = spinnerOf(t)
	for i := 0; i < 50; i++ {
		s.Key("left")
	}
	if s.omega != -spinnerMaxOmega {
		t.Errorf("반시계 상한: %v", s.omega)
	}
}

func TestSpinnerStopsFromAnyMashing(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	keys := []string{" ", "space", "enter", "up", "down", "left", "right", "r", "x", "esc"}
	for trial := 0; trial < 50; trial++ {
		s := spinnerOf(t)
		for i, n := 0, rng.Intn(40); i < n; i++ {
			s.Key(keys[rng.Intn(len(keys))])
			if rng.Intn(3) == 0 {
				s.Step()
			}
		}
		if n := spinnerFramesToStop(s, 1000); n < 0 {
			t.Fatalf("시도 %d: 1000프레임 안에 멈추지 않음 (omega=%v)", trial, s.omega)
		}
		if s.omega != 0 {
			t.Errorf("시도 %d: 멈췄는데 omega=%v", trial, s.omega)
		}
		if s.Step() {
			t.Errorf("시도 %d: 멈춘 뒤 Step 이 true", trial)
		}
	}
}

func TestSpinnerStoppedStepIsNoop(t *testing.T) {
	s := spinnerOf(t)
	angle := s.angle
	if s.Step() || s.angle != angle || s.turns != 0 {
		t.Errorf("정지 상태에서 Step 은 아무것도 바꾸지 않아야 함")
	}
}

func TestSpinnerDecayTuning(t *testing.T) {
	s := spinnerOf(t)
	for i := 0; i < 10; i++ {
		s.Key(" ")
	}
	max := spinnerFramesToStop(s, 1000)
	if max < 120 || max > 300 {
		t.Errorf("최대 속도에서 멈출 때까지 %d프레임, 목표 120~300", max)
	}
	s = spinnerOf(t)
	s.Key(" ")
	one := spinnerFramesToStop(s, 1000)
	if one < 60 || one > 300 {
		t.Errorf("한 번 튕겼을 때 %d프레임, 목표 60~300", one)
	}
	if one >= max {
		t.Errorf("세게 튕긴 쪽이 더 오래 돌아야 함: 한 번=%d 최대=%d", one, max)
	}
	t.Logf("최대 속도 %d프레임(%.1fs), 한 번 튕김 %d프레임(%.1fs)", max, float64(max)*fidgetFrame.Seconds(), one, float64(one)*fidgetFrame.Seconds())
}

func TestSpinnerTurnsAccumulate(t *testing.T) {
	s := spinnerOf(t)
	s.Key(" ")
	n := spinnerFramesToStop(s, 1000)
	if n < 0 || s.turns < 1 || s.turns > 30 {
		t.Errorf("한 번 튕긴 누적 회전수가 이상함: %v", s.turns)
	}
	one := s.turns
	s.Key("left")
	spinnerFramesToStop(s, 1000)
	if s.turns <= one {
		t.Errorf("반시계도 회전수에 더해져야 함: %v -> %v", one, s.turns)
	}
	if want := spinnerStartAng; s.angle < -2*math.Pi || s.angle > 2*math.Pi || math.IsNaN(s.angle) {
		t.Errorf("각도가 범위를 벗어남: %v (시작 %v)", s.angle, want)
	}
}

func TestSpinnerRowsShape(t *testing.T) {
	check := func(s *spinnerMode, cw, ch int) {
		t.Helper()
		rows := s.Rows(cw, ch)
		if len(rows) != ch {
			t.Fatalf("%dx%d: %d줄을 돌려줌", cw, ch, len(rows))
		}
		for i, r := range rows {
			w := 0
			for _, g := range r {
				w += lipgloss.Width(g.t)
			}
			if w > cw {
				t.Errorf("%dx%d: %d번째 줄 폭 %d 이 cw 를 넘음", cw, ch, i, w)
			}
		}
	}
	states := []*spinnerMode{spinnerOf(t)}
	for _, keys := range [][]string{{" "}, {" ", " ", " ", " "}, {"left"}, {"left", "left", "left", "left", "left", "left", "left"}} {
		s := spinnerOf(t)
		for _, k := range keys {
			s.Key(k)
		}
		for i := 0; i < 7; i++ {
			s.Step()
		}
		states = append(states, s)
	}
	for _, s := range states {
		for _, sz := range testSizes {
			check(s, sz[0]-6-4, max(3, min(14, sz[1]-9)))
		}
		for _, cw := range []int{1, 2, 3, 7, 50, 200} {
			for _, ch := range []int{1, 2, 3, 14, 20} {
				check(s, cw, ch)
			}
		}
	}
	if rows := spinnerOf(t).Rows(10, 0); len(rows) != 0 {
		t.Errorf("ch=0 이면 빈 결과여야 함")
	}
	if rows := spinnerOf(t).Rows(0, 3); len(rows) != 3 {
		t.Errorf("cw=0 이어도 ch 줄이어야 함")
	}
}

func TestSpinnerDrawsAndRotates(t *testing.T) {
	s := spinnerOf(t)
	joined := func() string {
		var b strings.Builder
		for _, r := range s.Rows(40, 12) {
			for _, g := range r {
				b.WriteString(g.t)
			}
			b.WriteByte('\n')
		}
		return b.String()
	}
	a := joined()
	if strings.TrimSpace(a) == "" {
		t.Fatalf("스피너가 그려지지 않음")
	}
	s.angle += 0.5
	if joined() == a {
		t.Errorf("각도가 바뀌어도 그림이 같음")
	}
	s.angle -= 0.5
	if joined() != a {
		t.Errorf("같은 상태인데 그림이 다름")
	}
}

func TestSpinnerSymmetricEvery120Degrees(t *testing.T) {
	s := spinnerOf(t)
	base := s.Rows(40, 12)
	s.angle += 2 * math.Pi / 3
	turned := s.Rows(40, 12)
	diff := 0
	for i := range base {
		var x, y strings.Builder
		for _, g := range base[i] {
			x.WriteString(g.t)
		}
		for _, g := range turned[i] {
			y.WriteString(g.t)
		}
		if x.String() != y.String() {
			diff++
		}
	}
	if diff > 2 {
		t.Errorf("120도 돌린 그림이 거의 같아야 함 (다른 줄 %d)", diff)
	}
}

func TestSpinnerGhostsOnlyWhenFast(t *testing.T) {
	if spinnerGhosts(0) != 0 || spinnerGhosts(0.05) != 0 {
		t.Errorf("느리면 잔상이 없어야 함")
	}
	if spinnerGhosts(1.0) < 2 || spinnerGhosts(-1.0) != spinnerGhosts(1.0) {
		t.Errorf("빠르면 잔상이 있어야 하고 방향과 무관해야 함")
	}
}

func TestSpinnerKeyFuzzNoPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	keys := []string{" ", "space", "enter", "up", "down", "left", "right", "r", "R", "x", "", "tab", "ctrl+c", "한"}
	s := spinnerOf(t)
	for i := 0; i < 3000; i++ {
		switch rng.Intn(3) {
		case 0:
			s.Key(keys[rng.Intn(len(keys))])
		case 1:
			s.Step()
		default:
			s.Rows(1+rng.Intn(200), 1+rng.Intn(20))
			s.Status()
			s.Keys()
		}
		if math.IsNaN(s.omega) || math.IsNaN(s.angle) || math.Abs(s.omega) > spinnerMaxOmega {
			t.Fatalf("상태가 망가짐: %+v", *s)
		}
	}
}

func TestSpinnerResizeKeepsState(t *testing.T) {
	s := spinnerOf(t)
	s.Key(" ")
	s.Key("right")
	for i := 0; i < 20; i++ {
		s.Step()
	}
	before := *s
	for _, sz := range [][2]int{{10, 3}, {100, 14}, {1, 1}, {50, 7}} {
		s.Rows(sz[0], sz[1])
	}
	if *s != before {
		t.Errorf("그리기만 했는데 상태가 바뀜: %+v -> %+v", before, *s)
	}
}

func TestSpinnerStatusAndKeys(t *testing.T) {
	s := spinnerOf(t)
	if s.Title() != "스피너" {
		t.Errorf("탭 이름: %q", s.Title())
	}
	if got := s.Status(); !strings.Contains(got, "0.0바퀴") || !strings.Contains(got, "정지") {
		t.Errorf("처음 상태 문구: %q", got)
	}
	s.Key(" ")
	if got := s.Status(); strings.Contains(got, "정지") || !strings.Contains(got, "시계") {
		t.Errorf("도는 중 상태 문구: %q", got)
	}
	s.Key("left")
	s.Key("left")
	s.Key("left")
	if got := s.Status(); !strings.Contains(got, "반시계") {
		t.Errorf("반시계 상태 문구: %q", got)
	}
	s.turns = 12.44
	if got := s.Status(); !strings.Contains(got, "12.4바퀴") {
		t.Errorf("회전수 표기: %q", got)
	}
	levels := map[string]bool{}
	for _, w := range []float64{0, 0.05, 0.2, 0.5, 0.9, -0.9} {
		levels[spinnerLevel(w)] = true
	}
	if len(levels) != 5 {
		t.Errorf("속도 단계가 5가지로 나뉘어야 함: %v", levels)
	}
	if keys := s.Keys(); len(keys) == 0 {
		t.Errorf("키 안내가 비어 있음")
	} else {
		for _, kv := range keys {
			if kv[0] == "" || kv[1] == "" {
				t.Errorf("키 안내에 빈 항목: %v", kv)
			}
		}
	}
}

func TestSpinnerInHostAligned(t *testing.T) {
	a := newFidgetApp(&Env{}).(*fidgetApp)
	a.cur = 0
	a.Update(key(" "))
	a.Update(key(" "))
	for i := 0; i < 5; i++ {
		a.modes[0].Step()
	}
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		assertFrame(t, "스피너", c, a.View(c))
	}
	if !strings.Contains(plain(a.View(testCtx(80, 24))), "바퀴") {
		t.Errorf("상태 줄에 바퀴 수가 보이지 않음")
	}
}

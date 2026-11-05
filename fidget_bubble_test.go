package main

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func newBubbleForTest() *bubbleMode { return newBubbleMode().(*bubbleMode) }

func bubbleStepsToIdle(t *testing.T, m *bubbleMode) int {
	t.Helper()
	for n := 1; n <= 1000; n++ {
		if !m.Step() {
			return n
		}
	}
	t.Fatalf("Step 이 멈추지 않음")
	return 0
}

// bubbleLit 은 Rows 결과의 켜진 점 좌표를 점자 글자에서 되살림
func bubbleLit(rows [][]seg) map[[2]int]bool {
	lit := map[[2]int]bool{}
	for cy, row := range rows {
		cx := 0
		for _, s := range row {
			for _, r := range s.t {
				if r >= 0x2800 && r < 0x2900 {
					for j := 0; j < 4; j++ {
						for i := 0; i < 2; i++ {
							if (r-0x2800)&brailleBit[j][i] != 0 {
								lit[[2]int{cx*2 + i, cy*4 + j}] = true
							}
						}
					}
				}
				cx++
			}
		}
	}
	return lit
}

func bubbleComponents(lit map[[2]int]bool) int {
	seen := map[[2]int]bool{}
	n := 0
	for p := range lit {
		if seen[p] {
			continue
		}
		n++
		stack := [][2]int{p}
		seen[p] = true
		for len(stack) > 0 {
			q := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					nb := [2]int{q[0] + dx, q[1] + dy}
					if lit[nb] && !seen[nb] {
						seen[nb] = true
						stack = append(stack, nb)
					}
				}
			}
		}
	}
	return n
}

func bubbleFlat(rows [][]seg) string {
	var b strings.Builder
	for _, r := range rows {
		for _, s := range r {
			b.WriteString(s.t)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func bubbleCheckRows(t *testing.T, rows [][]seg, cw, ch int) {
	t.Helper()
	if len(rows) != ch {
		t.Fatalf("%dx%d: %d줄", cw, ch, len(rows))
	}
	for y, r := range rows {
		w := 0
		for _, s := range r {
			w += lipgloss.Width(s.t)
		}
		if w > cw {
			t.Fatalf("%dx%d 줄 %d: 폭 %d 가 넘침", cw, ch, y, w)
		}
	}
}

func TestBubbleSpacePopsAndAdvances(t *testing.T) {
	m := newBubbleForTest()
	if !m.Key(" ") {
		t.Fatalf("Space 가 반응하지 않음")
	}
	if !m.popped[0] || m.cursor != 1 || m.count != 1 {
		t.Errorf("첫 방울이 터지고 커서가 다음으로 가야 함: popped0=%v cursor=%d count=%d", m.popped[0], m.cursor, m.count)
	}
	for _, k := range []string{"enter", "space"} {
		before := m.count
		if !m.Key(k) || m.count != before+1 {
			t.Errorf("%q 로 방울이 터지지 않음", k)
		}
	}
	if m.cursor != 3 {
		t.Errorf("연속으로 누르면 행 우선으로 이어져야 함: cursor=%d", m.cursor)
	}
	if m.Status() != "터뜨림 3/40" {
		t.Errorf("Status: %q", m.Status())
	}
}

func TestBubbleSpaceOnPoppedIgnored(t *testing.T) {
	m := newBubbleForTest()
	m.Key(" ")
	bubbleStepsToIdle(t, m)
	m.Key("left")
	if m.cursor != 0 {
		t.Fatalf("왼쪽 이동 실패: %d", m.cursor)
	}
	before := *m
	if m.Key(" ") || m.Key("enter") {
		t.Errorf("터진 방울에 Space 가 반응함")
	}
	if *m != before {
		t.Errorf("터진 방울에 Space 가 상태를 바꿈")
	}
}

func TestBubbleCursorMoves(t *testing.T) {
	m := newBubbleForTest()
	for _, k := range []string{"left", "up"} {
		if m.Key(k) || m.cursor != 0 {
			t.Errorf("모서리에서 %s 가 움직임: cursor=%d", k, m.cursor)
		}
	}
	m.Key("right")
	m.Key("down")
	if m.cursor != bubbleCols+1 {
		t.Errorf("right, down 뒤 커서: %d", m.cursor)
	}
	for i := 0; i < 20; i++ {
		m.Key("right")
		m.Key("down")
	}
	if m.cursor != bubbleCount-1 {
		t.Errorf("오른쪽 아래 끝에서 멈춰야 함: %d", m.cursor)
	}
	if m.Key("right") || m.Key("down") {
		t.Errorf("끝에서 이동 키가 반응함")
	}
	m.Key("left")
	m.Key("up")
	if m.cursor != bubbleCount-1-1-bubbleCols {
		t.Errorf("left, up 뒤 커서: %d", m.cursor)
	}
	if m.Key("x") || m.Key("tab") {
		t.Errorf("모르는 키가 반응함")
	}
}

func TestBubbleCursorSkipsPoppedAndWraps(t *testing.T) {
	m := newBubbleForTest()
	m.cursor = 5
	m.Key(" ")
	m.cursor = 4
	m.Key(" ")
	if m.cursor != 6 {
		t.Errorf("터진 방울을 건너뛰어야 함: %d", m.cursor)
	}
	m.cursor = bubbleCount - 1
	m.Key(" ")
	if m.cursor != 0 {
		t.Errorf("끝에서 처음으로 이어져야 함: %d", m.cursor)
	}
}

func TestBubbleAnimationEndsAfterSixFrames(t *testing.T) {
	m := newBubbleForTest()
	if m.Step() {
		t.Errorf("아무것도 안 했는데 Step 이 true")
	}
	m.Key(" ")
	if n := bubbleStepsToIdle(t, m); n != bubbleAnimFrames {
		t.Errorf("터지는 애니메이션이 %d 프레임(목표 %d)", n, bubbleAnimFrames)
	}
	if bubbleAnimFrames > 6 {
		t.Errorf("애니메이션은 최대 6프레임")
	}
}

func TestBubbleStepStopsAfterAnyKeyMash(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	keys := []string{" ", "enter", "space", "left", "right", "up", "down", "r", "x"}
	m := newBubbleForTest()
	for i := 0; i < 500; i++ {
		m.Key(keys[rng.Intn(len(keys))])
		if rng.Intn(3) == 0 {
			m.Step()
		}
		probe := *m
		if n := bubbleStepsToIdle(t, &probe); n > bubbleAnimFrames {
			t.Fatalf("키 입력 뒤 %d 프레임 넘게 움직임", n)
		}
	}
}

func TestBubblePopAllInFortyPresses(t *testing.T) {
	m := newBubbleForTest()
	for i := 0; i < bubbleCount; i++ {
		if !m.Key(" ") {
			t.Fatalf("%d번째 Space 가 무시됨", i+1)
		}
		m.Step()
	}
	if m.count != bubbleCount {
		t.Fatalf("다 터지지 않음: %d", m.count)
	}
	if m.Key(" ") {
		t.Errorf("다 터진 뒤 Space 가 반응함")
	}
	if !strings.Contains(m.Status(), "다 터뜨림! r 로 새 판") || !strings.Contains(m.Status(), "40/40") {
		t.Errorf("다 터진 Status: %q", m.Status())
	}
	bubbleStepsToIdle(t, m)
	if m.Step() {
		t.Errorf("다 터지고 애니메이션이 끝났는데 Step 이 true")
	}
}

func TestBubbleResetKey(t *testing.T) {
	m := newBubbleForTest()
	for i := 0; i < 7; i++ {
		m.Key(" ")
	}
	m.Key("down")
	if !m.Key("r") {
		t.Errorf("r 이 반응하지 않음")
	}
	if *m != *newBubbleForTest() {
		t.Errorf("r 뒤에 처음 상태여야 함: %+v", *m)
	}
	if m.Step() {
		t.Errorf("새 판 뒤에 애니메이션이 남음")
	}
}

func TestBubbleStatusAndKeys(t *testing.T) {
	m := newBubbleForTest()
	if m.Title() != "뽁뽁이" {
		t.Errorf("Title: %q", m.Title())
	}
	if m.Status() != "터뜨림 0/40" {
		t.Errorf("처음 Status: %q", m.Status())
	}
	keys := m.Keys()
	if len(keys) != 3 {
		t.Fatalf("Keys: %v", keys)
	}
	for _, k := range keys {
		if k[0] == "" || k[1] == "" {
			t.Errorf("빈 키 안내: %v", k)
		}
	}
}

func TestBubbleRowsShapeAllSizes(t *testing.T) {
	m := newBubbleForTest()
	for i := 0; i < 12; i++ {
		m.Key(" ")
	}
	var sizes [][2]int
	for _, sz := range testSizes {
		sizes = append(sizes, [2]int{sz[0] - 10, min(14, max(3, sz[1]-9))})
	}
	for _, cw := range []int{1, 3, 200} {
		for _, ch := range []int{1, 3, 20} {
			sizes = append(sizes, [2]int{cw, ch})
		}
	}
	for step := 0; step < 8; step++ {
		for _, sz := range sizes {
			bubbleCheckRows(t, m.Rows(sz[0], sz[1]), sz[0], sz[1])
		}
		m.Step()
	}
}

func TestBubbleRandomKeysAndSizesNoPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	keys := []string{" ", "enter", "space", "left", "right", "up", "down", "r", "R", "x", "tab", ""}
	m := newBubbleForTest()
	for i := 0; i < 3000; i++ {
		m.Key(keys[rng.Intn(len(keys))])
		if rng.Intn(2) == 0 {
			m.Step()
		}
		cw, ch := 1+rng.Intn(200), 1+rng.Intn(20)
		bubbleCheckRows(t, m.Rows(cw, ch), cw, ch)
	}
}

func TestBubbleBubblesDistinctAtMinSize(t *testing.T) {
	if fidgetBlockGlyphs {
		t.Skip("블록 문자 모드에서는 점 단위 검사를 건너뜀")
	}
	m := newBubbleForTest()
	for _, sz := range [][2]int{{46, 8}, {60, 11}, {120, 14}} {
		if n := bubbleComponents(bubbleLit(m.Rows(sz[0], sz[1]))); n != bubbleCount {
			t.Errorf("%dx%d: 방울이 %d덩어리로 보임(목표 %d)", sz[0], sz[1], n, bubbleCount)
		}
	}
}

func TestBubblePoppedLooksDifferent(t *testing.T) {
	if fidgetBlockGlyphs {
		t.Skip("블록 문자 모드에서는 점 단위 검사를 건너뜀")
	}
	m := newBubbleForTest()
	fresh := len(bubbleLit(m.Rows(46, 8)))
	m.Key(" ")
	bubbleStepsToIdle(t, m)
	lit := bubbleLit(m.Rows(46, 8))
	if fresh-len(lit) < 10 {
		t.Errorf("터진 방울이 눈에 띄게 납작해야 함: 점 %d -> %d", fresh, len(lit))
	}
	if bubbleComponents(lit) != bubbleCount {
		t.Errorf("터진 자리에도 흔적(납작한 점)이 남아야 함")
	}
}

func TestBubbleAnimationFramesDiffer(t *testing.T) {
	m := newBubbleForTest()
	m.Key(" ")
	seen := map[string]bool{}
	for i := 0; i < bubbleAnimFrames; i++ {
		seen[bubbleFlat(m.Rows(46, 8))] = true
		m.Step()
	}
	seen[bubbleFlat(m.Rows(46, 8))] = true
	if want := map[bool]int{false: 5, true: 3}[fidgetBlockGlyphs]; len(seen) < want {
		t.Errorf("애니메이션 프레임이 서로 달라야 함: 서로 다른 화면 %d개", len(seen))
	}
}

func TestBubbleCursorIsAccent(t *testing.T) {
	m := newBubbleForTest()
	count := func() (accent, other int) {
		for _, r := range m.Rows(46, 8) {
			for _, s := range r {
				if strings.TrimSpace(s.t) == "" {
					continue
				}
				if s.fg == colorAccent {
					accent++
				} else {
					other++
				}
			}
		}
		return
	}
	if a, o := count(); a == 0 || o == 0 {
		t.Fatalf("커서 방울만 accent 여야 함: accent=%d other=%d", a, o)
	}
	before := bubbleFlat(m.Rows(46, 8))
	m.Key("right")
	if a, _ := count(); a == 0 {
		t.Errorf("커서를 옮겨도 accent 가 보여야 함")
	}
	if bubbleFlat(m.Rows(46, 8)) != before {
		t.Errorf("커서 이동은 모양이 아니라 색만 바꿔야 함")
	}
}

func TestBubbleStateSurvivesResize(t *testing.T) {
	m := newBubbleForTest()
	for i := 0; i < 9; i++ {
		m.Key(" ")
	}
	m.Key("right")
	before := *m
	small := bubbleFlat(m.Rows(46, 8))
	for _, sz := range [][2]int{{200, 14}, {3, 3}, {1, 1}, {80, 5}} {
		m.Rows(sz[0], sz[1])
	}
	if *m != before {
		t.Errorf("그리기만 했는데 상태가 바뀜")
	}
	if bubbleFlat(m.Rows(46, 8)) != small {
		t.Errorf("크기를 바꿨다 돌아오면 같은 그림이어야 함")
	}
	if m.Status() != "터뜨림 9/40" {
		t.Errorf("Status: %q", m.Status())
	}
}

func TestBubbleHostIntegration(t *testing.T) {
	a := newFidgetApp(&Env{}).(*fidgetApp)
	a.Update(key("tab"))
	a.Update(key("tab"))
	if a.modes[a.cur].Title() != "뽁뽁이" {
		t.Fatalf("세 번째 탭이 뽁뽁이가 아님: %s", a.modes[a.cur].Title())
	}
	tick := appTickMsg{Gen: a.gen, Tag: a.tag()}
	for i := 0; i < 20 && a.ticking; i++ {
		a.Update(tick)
	}
	if _, cmd := a.Update(key(" ")); cmd == nil || !a.ticking {
		t.Errorf("Space 뒤 틱이 시작돼야 함")
	}
	for i := 0; i < 20 && a.ticking; i++ {
		a.Update(tick)
	}
	if a.ticking {
		t.Errorf("애니메이션이 끝나도 틱이 멈추지 않음")
	}
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		assertFrame(t, "뽁뽁이", c, a.View(c))
	}
	if !strings.Contains(plain(a.View(testCtx(80, 24))), "터뜨림 1/40") {
		t.Errorf("상태 줄에 터뜨림 횟수가 보이지 않음")
	}
}

func TestBubbleGeomAlignedToCells(t *testing.T) {
	for cw := 20; cw <= 200; cw++ {
		for ch := 4; ch <= 20; ch++ {
			g := newBubbleGeom(cw*2, ch*4)
			if g.p < 4 {
				continue
			}
			if g.p%4 != 0 || g.ox%2 != 0 || g.oy%4 != 0 {
				t.Fatalf("%dx%d: 방울이 칸 경계에 안 맞음 p=%d ox=%d oy=%d", cw, ch, g.p, g.ox, g.oy)
			}
			if g.ox+g.p*bubbleCols > cw*2 || g.oy+g.p*bubbleRows > ch*4 {
				t.Fatalf("%dx%d: 격자가 캔버스를 넘침", cw, ch)
			}
		}
	}
}

func TestBubbleAccentStaysInCursorBubble(t *testing.T) {
	if fidgetBlockGlyphs {
		t.Skip("블록 문자 모드에서는 점 단위 검사를 건너뜀")
	}
	sizes := [][2]int{{50, 11}, {46, 9}, {60, 11}, {46, 8}, {56, 17}, {80, 21}, {80, 24}, {200, 20}}
	for cw := 46; cw <= 120; cw += 7 {
		for ch := 8; ch <= 20; ch++ {
			sizes = append(sizes, [2]int{cw, ch})
		}
	}
	for _, sz := range sizes {
		cw, ch := sz[0], sz[1]
		g := newBubbleGeom(cw*2, ch*4)
		for cur := 0; cur < bubbleCount; cur++ {
			for _, popped := range []bool{false, true} {
				m := newBubbleForTest()
				m.cursor = cur
				m.popped[cur] = popped
				x0, y0 := g.ox+cur%bubbleCols*g.p, g.oy+cur/bubbleCols*g.p
				for cy, row := range m.Rows(cw, ch) {
					cx := 0
					for _, s := range row {
						for _, r := range s.t {
							if s.fg == colorAccent && r >= 0x2801 && r < 0x2900 {
								for j := 0; j < 4; j++ {
									for i := 0; i < 2; i++ {
										x, y := cx*2+i, cy*4+j
										if (r-0x2800)&brailleBit[j][i] != 0 && (x < x0 || x >= x0+g.p || y < y0 || y >= y0+g.p) {
											t.Fatalf("%dx%d 커서 %d(터짐 %v): accent 가 커서 방울 밖 점 (%d,%d) 에 번짐", cw, ch, cur, popped, x, y)
										}
									}
								}
							}
							cx++
						}
					}
				}
			}
		}
	}
}

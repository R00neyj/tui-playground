package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// 모든 테스트가 실제 사용자 설정 파일을 읽거나 쓰지 않게 임시 경로로 돌림
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "tui-settings-")
	if err != nil {
		panic(err)
	}
	settingsPath = func() string { return filepath.Join(dir, "settings.json") }
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func forceProfile(t *testing.T, p termenv.Profile) {
	t.Helper()
	old := termProfile()
	oldL := lipgloss.ColorProfile()
	setTermProfile(p)
	lipgloss.SetColorProfile(p)
	t.Cleanup(func() { setTermProfile(old); lipgloss.SetColorProfile(oldL) })
}

func lineWidths(s string) (n int, ok bool, w int) {
	lines := strings.Split(s, "\n")
	w = ansi.StringWidth(lines[0])
	for _, l := range lines {
		if ansi.StringWidth(l) != w {
			return len(lines), false, w
		}
	}
	return len(lines), true, w
}

var allProfiles = []termenv.Profile{termenv.TrueColor, termenv.ANSI256, termenv.ANSI}

func TestGridRoundTrip(t *testing.T) {
	for _, p := range allProfiles {
		forceProfile(t, p)
		c := testCtx(80, 24)
		box := NewFrame(c).Blank().Row(sgBold("한글 제목 abc", colorAccent), sgb(" 배지 ", colorOnState, colorOk)).Blank().String()
		bw, bh := boxSize(box)
		g := parseGrid(box, bw, bh)
		out := g.String()
		if ansi.Strip(out) != ansi.Strip(box) {
			t.Fatalf("profile %v: 글자가 달라짐\n%q\n%q", p, ansi.Strip(out), ansi.Strip(box))
		}
		if n, ok, w := lineWidths(out); !ok || w != bw || n != bh {
			t.Fatalf("profile %v: 폭/줄 수가 달라짐 n=%d ok=%v w=%d (기대 %d x %d)", p, n, ok, w, bh, bw)
		}
		// 색이 살아 있는지: 배지 칸의 배경 조각이 남아야 함
		if p != termenv.Ascii && !strings.Contains(out, "\x1b[") {
			t.Fatalf("profile %v: 색 정보가 사라짐", p)
		}
	}
}

func TestBackgroundsRender(t *testing.T) {
	for _, p := range allProfiles {
		forceProfile(t, p)
		for _, k := range bgKinds {
			for _, sz := range testSizes {
				for _, tm := range []float64{0, 1.7, 13.3} {
					g := newGrid(sz[0], sz[1])
					bgFill(g, k.ID, tm)
					n, ok, w := lineWidths(g.String())
					if !ok || w != sz[0] || n != sz[1] {
						t.Fatalf("배경 %s %v %dx%d t=%v: 폭/줄 수 어긋남 (n=%d w=%d)", k.ID, p, sz[0], sz[1], tm, n, w)
					}
				}
			}
		}
	}
}

func TestTransitionsEndpointsAndWidth(t *testing.T) {
	forceProfile(t, termenv.TrueColor)
	c := testCtx(80, 24)
	a := parseGrid(c.Place(NewFrame(c).Blank().Row(sg("옛 화면 한글 AAA", colorText)).Blank().String()), c.W, c.H)
	b := parseGrid(c.Place(NewFrame(c).Blank().Row(sg("새 화면 BBB 가나다", colorText)).Blank().Row(sg("둘째 줄", colorText)).String()), c.W, c.H)
	for _, k := range transKinds {
		for _, rev := range []bool{false, true} {
			at0 := ansi.Strip(blend(k.ID, a, b, 0, rev).String())
			at1 := ansi.Strip(blend(k.ID, a, b, 1, rev).String())
			if at0 != ansi.Strip(a.String()) {
				t.Errorf("전환 %d rev=%v: u=0 이 옛 화면과 다름", k.ID, rev)
			}
			if at1 != ansi.Strip(b.String()) {
				t.Errorf("전환 %d rev=%v: u=1 이 새 화면과 다름", k.ID, rev)
			}
			for i := 0; i <= 20; i++ {
				out := blend(k.ID, a, b, float64(i)/20, rev).String()
				n, ok, w := lineWidths(out)
				if !ok || w != c.W || n != c.H {
					t.Fatalf("전환 %d rev=%v u=%d/20: 폭/줄 수 어긋남 n=%d w=%d", k.ID, rev, i, n, w)
				}
			}
		}
	}
}

func TestSettingsLoadSaveCycle(t *testing.T) {
	p := settingsPath()
	os.Remove(p)
	if got := loadSettings(); got != defaultSettings {
		t.Fatalf("파일이 없으면 기본값이어야 함: %+v", got)
	}
	s := Settings{Bg: "C", Trans: 4, Arcade: true}
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(); got != s {
		t.Fatalf("저장한 값이 그대로 읽혀야 함: %+v", got)
	}
	// 배경 끔("")과 전환 끔(0)은 기본값으로 되돌아가면 안 됨
	off := Settings{Bg: "", Trans: 0, Arcade: false}
	if err := off.save(); err != nil {
		t.Fatal(err)
	}
	if got := loadSettings(); got != off {
		t.Fatalf("끔 값이 유지돼야 함: %+v", got)
	}
	// 이상한 값은 항목 단위로 기본값
	os.WriteFile(p, []byte(`{"bg":"Z","trans":9,"arcade":true}`), 0o600)
	if got := loadSettings(); got.Bg != defaultSettings.Bg || got.Trans != defaultSettings.Trans || !got.Arcade {
		t.Fatalf("잘못된 값만 기본값이어야 함: %+v", got)
	}
	os.WriteFile(p, []byte(`not json`), 0o600)
	if got := loadSettings(); got != defaultSettings {
		t.Fatalf("깨진 파일은 기본값: %+v", got)
	}
	// 한 바퀴 돌면 처음으로 돌아옴
	c := Settings{Bg: ""}
	for i := 0; i <= len(bgKinds); i++ {
		c = c.cycle(setBg, 1)
	}
	if c.Bg != "" {
		t.Fatalf("배경이 한 바퀴 돌아 끔으로 와야 함: %q", c.Bg)
	}
	if got := (Settings{Trans: 0}).cycle(setTrans, -1); got.Trans != len(transKinds) {
		t.Fatalf("전환 거꾸로 돌기: %d", got.Trans)
	}
}

func fxModel(t *testing.T, w, h int, s Settings) model {
	t.Helper()
	forceProfile(t, termenv.TrueColor)
	m := newModel()
	m.set = s
	m.ph = phHome
	m.w, m.h = w, h
	return m
}

func TestHomeCompositeAligned(t *testing.T) {
	for _, k := range bgKinds {
		for _, arcade := range []bool{false, true} {
			for _, sz := range testSizes {
				m := fxModel(t, sz[0], sz[1], Settings{Bg: k.ID, Trans: 3, Arcade: arcade})
				v := m.View()
				n, ok, w := lineWidths(v)
				if !ok || w != sz[0] || n != sz[1] {
					t.Fatalf("배경 %s 조작부=%v %dx%d: 폭/줄 수 어긋남 (n=%d ok=%v w=%d)", k.ID, arcade, sz[0], sz[1], n, ok, w)
				}
				if !strings.Contains(ansi.Strip(v), appName) {
					t.Fatalf("배경 %s %dx%d: 박스 제목이 안 보임", k.ID, sz[0], sz[1])
				}
			}
		}
	}
}

func TestArcadeAutoHidesWhenSmall(t *testing.T) {
	big := fxModel(t, 100, 40, Settings{Arcade: true})
	if !big.arcadeFits(big.ctx()) || !strings.Contains(ansi.Strip(big.View()), "COIN") {
		t.Fatal("큰 창에서는 조작부가 보여야 함")
	}
	small := fxModel(t, 80, 21, Settings{Arcade: true})
	if small.arcadeFits(small.ctx()) || strings.Contains(ansi.Strip(small.View()), "COIN") {
		t.Fatal("작은 창에서는 조작부가 숨겨져야 함")
	}
	off := fxModel(t, 100, 40, Settings{Arcade: false})
	if strings.Contains(ansi.Strip(off.View()), "COIN") {
		t.Fatal("끔이면 조작부가 없어야 함")
	}
}

func TestArcadeJoystickAndButtonsMove(t *testing.T) {
	forceProfile(t, termenv.TrueColor)
	now := time.Now()
	var idle, left, up, pressed pulses
	left.press(pLeft, now.Add(-200*time.Millisecond), time.Second)
	up.press(pUp, now.Add(-200*time.Millisecond), time.Second)
	pressed.press(pA, now.Add(-100*time.Millisecond), time.Second)
	pressed.press(pNum+2, now.Add(-100*time.Millisecond), time.Second)
	s0 := ansi.Strip(arcadeGrid(52, 7, idle, now, "B 종료").String())
	if ansi.Strip(arcadeGrid(52, 7, left, now, "B 종료").String()) == s0 {
		t.Error("왼쪽을 누르면 조이스틱 모양이 바뀌어야 함")
	}
	if ansi.Strip(arcadeGrid(52, 7, up, now, "B 종료").String()) == s0 {
		t.Error("위쪽을 누르면 조이스틱 모양이 바뀌어야 함")
	}
	g0 := arcadeGrid(52, 7, idle, now, "B 종료").String()
	g1 := arcadeGrid(52, 7, pressed, now, "B 종료").String()
	if g0 == g1 {
		t.Error("버튼을 누르면 모양이나 색이 바뀌어야 함")
	}
	for _, g := range []string{g0, g1} {
		if n, ok, w := lineWidths(g); !ok || w != 52 || n != arcadeH {
			t.Errorf("조작부 크기가 어긋남 n=%d ok=%v w=%d", n, ok, w)
		}
	}
	// 시간이 지나면 원래대로 돌아옴
	if arcadeGrid(52, 7, left, now.Add(3*time.Second), "B 종료").String() != g0 {
		t.Error("키를 뗀 뒤에는 조이스틱이 제자리로 돌아와야 함 (코인 깜박임 제외한 모양 비교)")
	}
}

func fxKey(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func step(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	nm, _ := m.Update(msg)
	return nm.(model)
}

func TestSettingsScreenFlow(t *testing.T) {
	os.Remove(settingsPath())
	m := fxModel(t, 100, 40, Settings{Bg: "A", Trans: 1, Arcade: false})
	m = step(t, m, fxKey("s"))
	if m.ph != phSettings || !strings.Contains(ansi.Strip(m.View()), "홈 배경") {
		t.Fatal("s 로 설정 화면이 열려야 함")
	}
	m = step(t, m, fxKey("right"))
	if m.set.Bg != "B" {
		t.Fatalf("→ 로 배경이 바뀌어야 함: %q", m.set.Bg)
	}
	m = step(t, m, fxKey("down"))
	m = step(t, m, fxKey("right"))
	if m.set.Trans != 2 {
		t.Fatalf("전환이 바뀌어야 함: %d", m.set.Trans)
	}
	m = step(t, m, fxKey("down"))
	m = step(t, m, fxKey("enter"))
	if !m.set.Arcade {
		t.Fatal("조작부를 켤 수 있어야 함")
	}
	if got := loadSettings(); got != m.set {
		t.Fatalf("바꾼 즉시 저장돼야 함: %+v vs %+v", got, m.set)
	}
	m = step(t, m, fxKey("esc"))
	if m.ph != phHome {
		t.Fatal("Esc 로 홈에 돌아와야 함")
	}
}

func TestTransitionOnOpenAndHome(t *testing.T) {
	m := fxModel(t, 100, 40, Settings{Bg: "H", Trans: 3})
	m = step(t, m, fxKey("1"))
	if m.ph != phApp || m.trans == nil {
		t.Fatalf("앱을 열면 전환이 시작돼야 함: ph=%v trans=%v", m.ph, m.trans)
	}
	if n, ok, w := lineWidths(m.View()); !ok || w != 100 || n != 40 {
		t.Fatalf("전환 중 화면 크기 어긋남 n=%d w=%d", n, w)
	}
	// 아무 키나 누르면 전환이 바로 끝남 (키는 앱에 전달됨)
	m = step(t, m, fxKey("x"))
	if m.trans != nil {
		t.Fatal("키 입력으로 전환이 끝나야 함")
	}
	// 시간이 지나면 틱이 전환을 끝냄
	m = step(t, m, goHomeMsg{})
	if m.ph != phHome || m.trans == nil || !m.trans.reverse {
		t.Fatal("홈으로 돌아올 때도 전환이 있어야 함")
	}
	m.trans.started = time.Now().Add(-2 * transDuration(m.trans.kind))
	m = step(t, m, animTickMsg(time.Now()))
	if m.trans != nil {
		t.Fatal("전환 시간이 지나면 끝나야 함")
	}
	// 전환을 끄면 즉시 바뀜
	off := fxModel(t, 100, 40, Settings{Bg: "H", Trans: 0})
	off = step(t, off, fxKey("1"))
	if off.trans != nil || off.ph != phApp {
		t.Fatal("전환 끔이면 바로 앱이 열려야 함")
	}
}

func TestEffectsOffWithoutColor(t *testing.T) {
	forceProfile(t, termenv.Ascii)
	m := newModel()
	m.set = Settings{Bg: "H", Trans: 3, Arcade: true}
	m.ph, m.w, m.h = phHome, 100, 40
	if m.fxHome(m.ctx()) || m.wantsAnim(time.Now()) {
		t.Fatal("색을 못 쓰는 터미널에서는 배경·전환이 꺼져야 함")
	}
	m = step(t, m, fxKey("1"))
	if m.trans != nil {
		t.Fatal("색을 못 쓰는 터미널에서는 전환이 없어야 함")
	}
	t.Setenv("TUI_ANIM", "off")
	forceProfile(t, termenv.TrueColor)
	if animEnabled() {
		t.Fatal("TUI_ANIM=off 면 꺼져야 함")
	}
}

func TestFrameBytes(t *testing.T) {
	m := fxModel(t, 120, 40, Settings{Bg: "H"})
	n := len(m.View())
	t.Logf("H 배경 한 프레임 크기: %d 바이트 (120x40, 트루컬러)", n)
	if n > 400000 {
		t.Errorf("한 프레임이 너무 큼: %d", n)
	}
}

// FX_DUMP=폴더 를 주면 설정별 화면을 ANSI 문자열 파일로 저장함 (눈으로 확인할 때만 씀)
func TestDumpFrames(t *testing.T) {
	dir := os.Getenv("FX_DUMP")
	if dir == "" {
		t.Skip("FX_DUMP 없음")
	}
	cases := map[string]Settings{"H": {Bg: "H", Arcade: true}, "A": {Bg: "A"}, "C": {Bg: "C"}, "E": {Bg: "E"}, "B": {Bg: "B"}}
	for name, s := range cases {
		m := fxModel(t, 100, 34, s)
		m.start = time.Now().Add(-3 * time.Second)
		if name == "H" {
			m = step(t, m, fxKey("down"))
			m = step(t, m, fxKey("right"))
			m = step(t, m, fxKey("enter"))
			m.ph = phSettings
			os.WriteFile(filepath.Join(dir, "S.ansi"), []byte(m.View()), 0o600)
			m.ph = phHome
		}
		os.WriteFile(filepath.Join(dir, name+".ansi"), []byte(m.View()), 0o600)
	}
}

// 앱 화면에도 배경·조작부가 이어지는지: 모든 앱 × 배경 × 조작부 × 창 크기에서 줄 수와 폭이 맞아야 함
func TestAppScreensKeepBackgroundAndArcade(t *testing.T) {
	forceProfile(t, termenv.TrueColor)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	sizes := append([][2]int{{100, 40}}, testSizes...)
	for i := range registry {
		for _, arcade := range []bool{false, true} {
			for _, sz := range sizes {
				m := newModel()
				m.set = Settings{Bg: "H", Trans: 0, Arcade: arcade}
				m.w, m.h, m.ph = sz[0], sz[1], phHome
				mm, _ := m.openApp(i)
				v := mm.View()
				n, ok, w := lineWidths(v)
				if !ok || w != sz[0] || n != sz[1] {
					t.Fatalf("앱 %d 조작부=%v %dx%d: 폭/줄 수 어긋남 (n=%d ok=%v w=%d)", i, arcade, sz[0], sz[1], n, ok, w)
				}
				hasArcade := strings.Contains(ansi.Strip(v), "COIN")
				_, want := mm.appCtx(mm.ctx())
				if hasArcade != want {
					t.Fatalf("앱 %d %dx%d: 조작부 표시(%v)가 판단(%v)과 다름", i, sz[0], sz[1], hasArcade, want)
				}
				if sz[0] == 100 && sz[1] == 40 && arcade && !hasArcade {
					t.Fatalf("앱 %d: 큰 창에서는 조작부가 이어져야 함", i)
				}
			}
		}
	}
}

// 디졸브는 전환 중간(u=0.5)에 가장 짙고, 처음과 끝에서는 옅어야 함
func TestDissolvePeaksInMiddle(t *testing.T) {
	forceProfile(t, termenv.TrueColor)
	c := testCtx(100, 30)
	a := parseGrid(c.Place(NewFrame(c).Blank().Row(sg("옛 화면", colorText)).String()), c.W, c.H)
	b := parseGrid(c.Place(NewFrame(c).Blank().Row(sg("새 화면", colorText)).String()), c.W, c.H)
	dark := func(u float64) int {
		n := 0
		for _, cl := range blend(3, a, b, u, false).c {
			if cl.r == '▓' || cl.r == '█' {
				n++
			}
		}
		return n
	}
	d1, d5, d9 := dark(0.1), dark(0.5), dark(0.9)
	t.Logf("짙은 블록 수: u=0.1 → %d, u=0.5 → %d, u=0.9 → %d", d1, d5, d9)
	if d5 <= d1 || d5 <= d9 {
		t.Fatalf("중간이 가장 짙어야 함: %d %d %d", d1, d5, d9)
	}
}

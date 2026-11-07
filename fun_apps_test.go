package main

import (
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// 모래·생명 게임·불꽃놀이 모드와 반응속도·뱀·지뢰 찾기·결정 도구·피아노 앱의 핵심 동작 검사

func TestSandFallsAndSettles(t *testing.T) {
	m := newSandMode().(*sandMode)
	m.rng = rand.New(rand.NewSource(1))
	m.Key(" ")
	steps := 0
	for m.Step() {
		if steps++; steps > 2000 {
			t.Fatal("모래가 멈추지 않음")
		}
	}
	if m.count == 0 {
		t.Fatal("모래가 뿌려지지 않음")
	}
	n := 0
	for _, g := range m.grid {
		if g > 0 {
			n++
		}
	}
	if n != m.count {
		t.Errorf("모래 수가 맞지 않음: 격자 %d, 셈 %d", n, m.count)
	}
	for x := 0; x < sandW; x++ { // 떠 있는 모래가 없어야 함
		for y := 0; y < sandH-1; y++ {
			if m.at(x, y) > 0 && m.at(x, y+1) == 0 {
				t.Fatalf("(%d,%d) 모래가 공중에 떠 있음", x, y)
			}
		}
	}
	m.Key("r")
	if m.count != 0 {
		t.Errorf("r 로 비워지지 않음")
	}
}

func TestLifeBlinker(t *testing.T) {
	m := newLifeMode().(*lifeMode)
	m.Key("c")
	for _, x := range []int{10, 11, 12} {
		m.cells[m.idx(x, 5)] = true
	}
	m.Key("n")
	for _, y := range []int{4, 5, 6} {
		if !m.cells[m.idx(11, y)] {
			t.Fatalf("깜빡이가 세로로 바뀌지 않음 (11,%d)", y)
		}
	}
	if m.alive() != 3 || m.gen != 1 {
		t.Errorf("살아 있는 칸 %d, 세대 %d", m.alive(), m.gen)
	}
	if m.Step() {
		t.Errorf("멈춤 상태에서 Step 이 계속 돌면 안 됨")
	}
	m.Key("enter")
	if !m.Step() {
		t.Errorf("재생 중에는 Step 이 계속 돌아야 함")
	}
}

func TestFireworksExplodeAndFade(t *testing.T) {
	m := newFireworksMode().(*fireworksMode)
	m.rng = rand.New(rand.NewSource(2))
	m.Key(" ")
	exploded := false
	for i := 0; i < 1000 && m.Step(); i++ {
		if len(m.ps) > 1 {
			exploded = true
		}
	}
	if !exploded {
		t.Error("불꽃이 터지지 않음")
	}
	if len(m.ps) != 0 {
		t.Errorf("불티가 사라지지 않음: %d", len(m.ps))
	}
	for i := 0; i < 50; i++ {
		m.Key(" ")
	}
	if m.rockets() > fwMaxRocket {
		t.Errorf("로켓 상한을 넘음: %d", m.rockets())
	}
}

func TestReactionFlow(t *testing.T) {
	now := time.Unix(1000, 0)
	oldNow, oldRand := reactNow, reactRand
	reactNow, reactRand = func() time.Time { return now }, func(int) int { return 0 }
	t.Cleanup(func() { reactNow, reactRand = oldNow, oldRand })

	a := newReactionApp(&Env{}).(*reactionApp)
	a.Update(key(" "))
	if a.ph != reactWait {
		t.Fatalf("Space 뒤에 대기 상태가 아님: %v", a.ph)
	}
	a.Update(key(" "))
	if a.ph != reactEarly {
		t.Fatalf("대기 중에 누르면 너무 빠름이어야 함")
	}
	a.Update(appTickMsg{Gen: a.gen, Tag: "react1"}) // 무효가 된 옛 틱
	if a.ph != reactEarly {
		t.Fatalf("옛 틱이 상태를 바꿈")
	}
	a.Update(key(" "))
	a.Update(appTickMsg{Gen: a.gen, Tag: a.tag()})
	if a.ph != reactGo {
		t.Fatalf("틱 뒤에 지금 상태가 아님")
	}
	now = now.Add(234 * time.Millisecond)
	a.Update(key(" "))
	if a.ph != reactResult || a.last != 234 || a.best != 234 || !a.newBest {
		t.Fatalf("결과가 이상함: ph=%v last=%d best=%d", a.ph, a.last, a.best)
	}
	if v, ok := loadScore(reactScore); !ok || v != 234 {
		t.Errorf("최고 기록이 저장되지 않음: %d %v", v, ok)
	}
	if !strings.Contains(plain(a.View(testCtx(80, 24))), "234") {
		t.Errorf("결과가 화면에 없음")
	}
}

func TestSnakeEatsAndCrashes(t *testing.T) {
	oldRand := snakeRand
	snakeRand = func(int) int { return 0 }
	t.Cleanup(func() { snakeRand = oldRand })

	a := newSnakeApp(&Env{}).(*snakeApp)
	h := a.body[0]
	a.food = pt{h.x + 1, h.y}
	a.Update(key(" "))
	a.Update(appTickMsg{Gen: a.gen, Tag: a.tag()})
	if a.score != 1 || len(a.body) != 5 {
		t.Fatalf("먹이를 먹지 않음: 점수 %d 길이 %d", a.score, len(a.body))
	}
	a.Update(key("left")) // 반대 방향은 무시
	if len(a.queue) != 0 {
		t.Errorf("반대 방향 입력이 들어감")
	}
	for i := 0; i < snakeW && a.ph == snakeRun; i++ {
		a.Update(appTickMsg{Gen: a.gen, Tag: a.tag()})
	}
	if a.ph != snakeOver {
		t.Fatalf("벽에 부딪혀도 끝나지 않음")
	}
	if a.best < 1 {
		t.Errorf("최고 점수가 갱신되지 않음")
	}
	a.Update(key(" "))
	if a.ph != snakeRun || a.score != 0 {
		t.Errorf("끝난 뒤 Space 로 새 판이 시작되지 않음")
	}
}

func TestMinesFirstSafeAndWin(t *testing.T) {
	oldRand := minesRand
	minesRand = func(n int) int { return 0 }
	t.Cleanup(func() { minesRand = oldRand })

	a := newMinesApp(&Env{}).(*minesApp)
	a.cx, a.cy = minesW-1, minesH-1 // 지뢰는 앞쪽부터 놓임 (가짜 난수 0)
	a.Update(key(" "))
	if a.ph != minesPlay && a.ph != minesWon {
		t.Fatalf("첫 칸에서 지뢰를 밟음: %v", a.ph)
	}
	mines := 0
	for _, m := range a.mine {
		if m {
			mines++
		}
	}
	if mines != minesCount {
		t.Fatalf("지뢰 수 %d", mines)
	}
	if a.opened <= 1 {
		t.Errorf("빈 칸이 이어서 열리지 않음: %d", a.opened)
	}
	for i := range a.mine { // 남은 안전한 칸을 모두 열면 이김
		if !a.mine[i] && a.ph == minesPlay {
			a.reveal(i)
		}
	}
	if a.ph != minesWon {
		t.Fatalf("안전한 칸을 다 열어도 이기지 않음")
	}
	b := newMinesApp(&Env{}).(*minesApp)
	b.cx, b.cy = minesW-1, minesH-1
	b.Update(key(" "))
	b.reveal(0)
	if b.ph != minesLost {
		t.Errorf("지뢰를 열어도 지지 않음")
	}
	_ = b.View(testCtx(80, 24))
}

func TestLadderIsPermutation(t *testing.T) {
	for seed := int64(0); seed < 30; seed++ {
		r := rand.New(rand.NewSource(seed))
		old := decideRand
		decideRand = r.Intn
		a := newDecideApp(&Env{}).(*decideApp)
		for n := ladderMin; n <= ladderMax; n++ {
			a.ladN = n
			a.newLadder()
			seen := map[int]bool{}
			for s := 0; s < n; s++ {
				_, _, end := a.ladderPath(s)
				seen[end] = true
			}
			if !a.everyGapHasRung() {
				t.Fatalf("seed %d n %d: 가로줄이 없는 틈이 있음", seed, n)
			}
			if len(seen) != n {
				t.Fatalf("seed %d n %d: 도착점이 겹침", seed, n)
			}
			wins := 0
			for _, p := range a.prizes {
				if p == "당첨" {
					wins++
				}
			}
			if wins != 1 {
				t.Fatalf("당첨이 %d개", wins)
			}
		}
		decideRand = old
	}
}

func TestDecideDiceAndCoin(t *testing.T) {
	a := newDecideApp(&Env{}).(*decideApp)
	a.Update(key("up"))
	if len(a.dice) != 3 {
		t.Fatalf("주사위 개수가 늘지 않음")
	}
	a.Update(key(" "))
	for a.busy > 0 {
		a.Update(appTickMsg{Gen: a.gen, Tag: a.tag()})
	}
	for _, v := range a.dice {
		if v < 1 || v > 6 {
			t.Fatalf("주사위 눈이 이상함: %v", a.dice)
		}
	}
	if !strings.Contains(plain(a.View(testCtx(80, 24))), "합") {
		t.Errorf("주사위 합이 보이지 않음")
	}
	a.Update(key("tab"))
	a.Update(key(" "))
	for a.busy > 0 {
		a.Update(appTickMsg{Gen: a.gen, Tag: a.tag()})
	}
	if a.heads+a.tails != 1 {
		t.Errorf("동전 횟수가 맞지 않음")
	}
	a.Update(key("tab"))
	a.Update(key("a"))
	if v := plain(a.View(testCtx(80, 24))); !strings.Contains(v, "당첨") {
		t.Errorf("모두 열기 뒤 당첨이 보이지 않음:\n%s", v)
	}
}

func TestPianoPlaysNote(t *testing.T) {
	var got []float64
	old := pianoTone
	pianoTone = func(f float64, d time.Duration) { got = append(got, f) }
	t.Cleanup(func() { pianoTone = old })

	a := newPianoApp(&Env{}).(*pianoApp)
	a.Update(key("a"))
	a.Update(key("x"))
	a.Update(key("h"))
	if len(got) != 2 {
		t.Fatalf("음이 %d개 남", len(got))
	}
	if math.Abs(got[0]-261.63) > 0.1 || math.Abs(got[1]-880) > 0.1 {
		t.Errorf("주파수가 이상함: %v", got)
	}
	if !a.isLit(9) {
		t.Errorf("누른 건반이 빛나지 않음")
	}
	if a.Update(key("1")); len(got) != 2 {
		t.Errorf("건반이 아닌 키에 소리가 남")
	}
}

func TestPianoMixerChord(t *testing.T) {
	var m pianoMixer
	now := time.Unix(100, 0)
	one := m.press(261.63, now)
	if string(one[0:4]) != "RIFF" || string(one[8:12]) != "WAVE" || string(one[36:40]) != "data" {
		t.Fatalf("WAV 머리가 이상함")
	}
	if n := int(one[40]) | int(one[41])<<8 | int(one[42])<<16; n != len(one)-44 {
		t.Errorf("data 길이 %d, 실제 %d", n, len(one)-44)
	}
	m.press(329.63, now.Add(10*time.Millisecond))
	m.press(392.0, now.Add(20*time.Millisecond))
	if len(m.notes) != 3 {
		t.Fatalf("동시에 누른 세 음이 섞이지 않음: %d", len(m.notes))
	}
	m.press(440, now.Add(2*pianoLife))
	if len(m.notes) != 1 {
		t.Errorf("다 울린 음이 빠지지 않음: %d", len(m.notes))
	}
	for i := 0; i < 30; i++ {
		m.press(440, now.Add(2*pianoLife))
	}
	if len(m.notes) != pianoMaxN {
		t.Errorf("음 상한을 넘음: %d", len(m.notes))
	}
}

func TestHangulKeysMapToQwerty(t *testing.T) {
	cases := map[string]string{"ㅁ": "a", "마": "ak", "닭": "ekfr", "ㅘ": "hk", "ㅆ": "T", "a": "a"}
	for in, want := range cases {
		ks, ok := hangulKeys(runeKey(in))
		if got := strings.Join(ks, ""); got != want || ok != (in != "a") {
			t.Errorf("%q -> %q (%v), 기대 %q", in, got, ok, want)
		}
	}
	var got []float64
	old := pianoTone
	pianoTone = func(f float64, d time.Duration) { got = append(got, f) }
	t.Cleanup(func() { pianoTone = old })
	a := newPianoApp(&Env{}).(*pianoApp)
	a.Update(runeKey("ㅁ")) // 한글 상태의 a = 도
	a.Update(runeKey("마")) // 입력기가 조합한 글자 = a + k (도 + 높은 도)
	if len(got) != 3 || math.Abs(got[0]-261.63) > 0.1 {
		t.Fatalf("한글 입력이 건반으로 바뀌지 않음: %v", got)
	}
	if !strings.Contains(plain(a.View(testCtx(80, 24))), "한/영") {
		t.Errorf("한글 입력 안내가 보이지 않음")
	}
	s := newSnakeApp(&Env{}).(*snakeApp)
	s.Update(runeKey("ㄴ")) // 한글 상태의 s = 아래
	if s.ph != snakeRun {
		t.Errorf("뱀 게임이 한글 상태의 방향키(ㄴ)에 반응하지 않음")
	}
}

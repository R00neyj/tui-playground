package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withLunch 는 메뉴 파일을 테스트 전용 임시 경로로 돌리고, 룰렛이 고를 칸을 고정함
func withLunch(t *testing.T, target int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "lunch.json")
	oldPath, oldRand := lunchPath, lunchRand
	lunchPath = func() string { return p }
	lunchRand = func(n int) int { return target % n }
	t.Cleanup(func() { lunchPath, lunchRand = oldPath, oldRand })
	return p
}

func newLunch(t *testing.T) *lunchApp {
	t.Helper()
	return newLunchApp(&Env{}).(*lunchApp)
}

// spinOut 은 회전이 끝날 때까지 현재 회전의 틱을 직접 흘려 넣고, 필요했던 틱 수를 돌려줌
func spinOut(t *testing.T, a *lunchApp) int {
	t.Helper()
	n := 0
	for a.ph == lunchSpinning {
		if n++; n > 10000 {
			t.Fatalf("회전이 끝나지 않음")
		}
		a.Update(appTickMsg{Gen: a.gen, Tag: a.tag()})
	}
	return n
}

func TestLunchSpinStopsOnTarget(t *testing.T) {
	for _, target := range []int{0, 1, 5, 11} {
		withLunch(t, target)
		a := newLunch(t)
		start := a.pos
		_, cmd := a.Update(key(" "))
		if cmd == nil || a.ph != lunchSpinning {
			t.Fatalf("Space 로 회전이 시작되지 않음")
		}
		n := len(a.items)
		ticks := spinOut(t, a)
		if a.pos != target || a.ph != lunchDone {
			t.Errorf("target=%d: pos=%d ph=%d", target, a.pos, a.ph)
		}
		if want := 2*n + (target-start+n)%n; ticks != want {
			t.Errorf("target=%d: 틱 %d번, 기대 %d번 (최소 두 바퀴)", target, ticks, want)
		}
		if !strings.Contains(a.msg, a.items[target]) {
			t.Errorf("결과 메시지에 메뉴 이름이 없음: %q", a.msg)
		}
	}
}

func TestLunchSpinSlowsDown(t *testing.T) {
	prev := spinDelay(0, 30)
	for s := 1; s <= 30; s++ {
		d := spinDelay(s, 30)
		if d < prev {
			t.Fatalf("간격이 줄어듦: step %d", s)
		}
		prev = d
	}
	if spinDelay(30, 30) <= 3*spinDelay(0, 30) {
		t.Errorf("끝으로 갈수록 충분히 느려지지 않음")
	}
}

func TestLunchSkipAndStaleTicks(t *testing.T) {
	withLunch(t, 7)
	a := newLunch(t)
	a.Update(key(" "))
	oldTag := a.tag()
	a.Update(key(" ")) // 도는 중 Space: 바로 결과
	if a.ph != lunchDone || a.pos != 7 {
		t.Fatalf("건너뛰기 결과가 target 과 다름: pos=%d ph=%d", a.pos, a.ph)
	}
	a.Update(appTickMsg{Gen: a.gen, Tag: oldTag})
	if a.pos != 7 || a.ph != lunchDone {
		t.Errorf("끝난 뒤 도착한 옛 틱이 상태를 바꿈")
	}
	a.Update(key(" ")) // 다시 돌리면 옛 회전의 틱은 무시돼야 함
	a.Update(appTickMsg{Gen: a.gen, Tag: oldTag})
	if a.step != 0 {
		t.Errorf("옛 회전의 틱이 새 회전에 섞임")
	}
	a.Update(appTickMsg{Gen: a.gen + 1, Tag: a.tag()})
	if a.step != 0 {
		t.Errorf("다른 인스턴스의 틱을 받아들임")
	}
}

func TestLunchAddDeletePersist(t *testing.T) {
	p := withLunch(t, 0)
	a := newLunch(t)
	base := len(a.items)

	a.Update(key("a"))
	if !a.adding {
		t.Fatalf("a 를 눌러도 입력 모드가 아님")
	}
	for _, r := range "마라탕" {
		a.Update(key(string(r)))
	}
	a.Update(key("enter"))
	if a.adding || len(a.items) != base+1 || a.items[base] != "마라탕" || a.pos != base {
		t.Fatalf("추가 결과가 이상함: %v pos=%d", a.items, a.pos)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("저장 파일이 없음: %v", err)
	}
	if got := loadLunchItems(); len(got) != base+1 || got[base] != "마라탕" {
		t.Errorf("다시 읽은 목록이 다름: %v", got)
	}

	a.Update(key("a")) // 중복
	for _, r := range "마라탕" {
		a.Update(key(string(r)))
	}
	a.Update(key("enter"))
	if !a.adding || len(a.items) != base+1 || a.msgTone != ToneDanger {
		t.Errorf("중복 메뉴가 거부되지 않음")
	}
	a.Update(key("esc"))
	if a.adding {
		t.Errorf("Esc 로 입력이 취소되지 않음")
	}

	a.Update(key("d")) // 방금 추가한 마지막 칸 삭제 -> 포인터가 처음으로
	if len(a.items) != base || a.pos != 0 {
		t.Errorf("삭제 결과가 이상함: len=%d pos=%d", len(a.items), a.pos)
	}
	if got := loadLunchItems(); len(got) != base {
		t.Errorf("삭제가 저장되지 않음: %v", got)
	}
}

func TestLunchEmptyList(t *testing.T) {
	withLunch(t, 0)
	a := newLunch(t)
	for len(a.items) > 0 {
		a.Update(key("d"))
	}
	if got := loadLunchItems(); len(got) != 0 {
		t.Fatalf("빈 목록이 저장되지 않고 기본값으로 돌아감: %v", got)
	}
	if _, cmd := a.Update(key(" ")); cmd != nil || a.ph == lunchSpinning {
		t.Errorf("메뉴가 없는데 회전이 시작됨")
	}
	a.Update(key("up"))
	a.Update(key("d"))
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		assertFrame(t, "lunch-empty", c, a.View(c))
	}
	if !strings.Contains(plain(a.View(testCtx(80, 24))), "등록된 메뉴 없음") {
		t.Errorf("빈 상태 안내 문구가 없음")
	}
}

func TestLunchLoadBroken(t *testing.T) {
	p := withLunch(t, 0)
	os.WriteFile(p, []byte("{깨진 json"), 0o600)
	if got := loadLunchItems(); len(got) != len(defaultLunchItems) {
		t.Errorf("깨진 파일이면 기본 메뉴를 써야 함: %v", got)
	}
	os.WriteFile(p, []byte(`{"items":["  라멘 ","","국밥"]}`), 0o600)
	if got := loadLunchItems(); len(got) != 2 || got[0] != "라멘" {
		t.Errorf("공백·빈 항목 정리가 안 됨: %v", got)
	}
}

func TestLunchFewItemsWheel(t *testing.T) {
	p := withLunch(t, 1)
	os.WriteFile(p, []byte(`{"items":["가","나","다"]}`), 0o600)
	a := newLunch(t)
	spinOut2 := func() {
		a.Update(key(" "))
		spinOut(t, a)
	}
	spinOut2()
	if a.items[a.pos] != "나" {
		t.Errorf("3개짜리 목록에서 결과가 다름: %s", a.items[a.pos])
	}
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		v := a.View(c)
		assertFrame(t, "lunch-few", c, v)
		if strings.Count(plain(v), "▸") != 1 {
			t.Errorf("선택 표시가 정확히 하나가 아님")
		}
	}
}

func TestLunchViewsAligned(t *testing.T) {
	withLunch(t, 3)
	a := newLunch(t)
	check := func(name string) {
		for _, sz := range testSizes {
			c := testCtx(sz[0], sz[1])
			v := a.View(c)
			assertFrame(t, name, c, v)
			if n := strings.Count(v, "\n") + 1; n > sz[1] {
				t.Errorf("%s: %dx%d 창에서 %d줄로 창 높이를 넘음", name, sz[0], sz[1], n)
			}
		}
	}
	check("lunch-idle")
	a.Update(key(" "))
	check("lunch-spinning")
	spinOut(t, a)
	check("lunch-done")
	a.Update(key("a"))
	check("lunch-adding")
}

func TestLunchEscBehavior(t *testing.T) {
	withLunch(t, 0)
	a := newLunch(t)
	if _, cmd := a.Update(key("esc")); cmd == nil {
		t.Errorf("처음 상태에서 Esc 가 홈으로 가지 않음")
	}
	a = newLunch(t)
	a.Update(key("a"))
	if _, cmd := a.Update(key("esc")); cmd != nil || a.adding {
		t.Errorf("입력 중 Esc 는 입력만 취소해야 함")
	}
	a.Update(key(" "))
	if _, cmd := a.Update(key("esc")); cmd == nil {
		t.Errorf("도는 중 Esc 가 홈으로 가지 않음")
	}
}

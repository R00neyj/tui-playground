package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// 다른 테스트(전체 앱 공통 검사 등)가 실제 사용자 설정 폴더를 건드리지 않게 기본 저장 위치를 막아 둠
func init() {
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("tuipg-todo-test-%d", os.Getpid()))
	todoPath = func() (string, error) { return filepath.Join(dir, "none", "todo.json"), nil }
}

func todoSetup(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "TUI-playground", "todo.json")
	old := todoPath
	todoPath = func() (string, error) { return p, nil }
	t.Cleanup(func() { todoPath = old })
	return p
}

func todoNew() *todoApp { return newTodoApp(&Env{}).(*todoApp) }

func todoKeys(a *todoApp, keys ...string) {
	for _, k := range keys {
		if k == "space" {
			a.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
			continue
		}
		a.Update(key(k))
	}
}

func todoType(a *todoApp, text string) {
	for _, r := range text {
		if r == ' ' {
			todoKeys(a, "space")
			continue
		}
		a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func todoAdd(a *todoApp, text string) {
	todoKeys(a, "a")
	todoType(a, text)
	todoKeys(a, "enter")
}

func todoTexts(a *todoApp) string {
	var s []string
	for _, it := range a.items {
		m := "-"
		if it.Done {
			m = "x"
		}
		s = append(s, m+it.Text)
	}
	return strings.Join(s, ",")
}

func TestTodoAddToggleDelete(t *testing.T) {
	todoSetup(t)
	a := todoNew()
	todoAdd(a, "첫째")
	todoAdd(a, "둘째")
	todoAdd(a, "셋째")
	if got := todoTexts(a); got != "-첫째,-둘째,-셋째" || a.sel != 2 {
		t.Fatalf("추가 결과: %s sel=%d", got, a.sel)
	}
	todoKeys(a, "up", "k", "space") // 첫째 선택 후 완료
	if got := todoTexts(a); got != "x첫째,-둘째,-셋째" {
		t.Fatalf("토글 결과: %s", got)
	}
	todoKeys(a, "space")
	if got := todoTexts(a); got != "-첫째,-둘째,-셋째" {
		t.Fatalf("다시 토글: %s", got)
	}
	todoKeys(a, "j", "d")
	if got := todoTexts(a); got != "-첫째,-셋째" || a.sel != 1 {
		t.Fatalf("삭제 결과: %s sel=%d", got, a.sel)
	}
	todoKeys(a, "d", "d", "d", "space", "up", "down") // 빈 목록에서도 멈추지 않음
	if len(a.items) != 0 || a.sel != 0 {
		t.Fatalf("전부 삭제: %s sel=%d", todoTexts(a), a.sel)
	}
}

func TestTodoAddInput(t *testing.T) {
	todoSetup(t)
	a := todoNew()
	todoAdd(a, "   ") // 공백만이면 추가하지 않음
	todoKeys(a, "a", "esc")
	if len(a.items) != 0 || a.adding {
		t.Fatalf("items=%v adding=%v", a.items, a.adding)
	}
	todoAdd(a, "  우유 사오기  ")
	if todoTexts(a) != "-우유 사오기" {
		t.Fatalf("앞뒤 공백이 남음: %q", todoTexts(a))
	}
	// 입력 중에는 d, j, space 가 글자로 들어가고 Esc 는 홈이 아니라 취소
	todoKeys(a, "a", "d", "j", "space")
	if a.input.Value() != "dj " || len(a.items) != 1 {
		t.Fatalf("입력값 %q items=%d", a.input.Value(), len(a.items))
	}
	if _, cmd := a.Update(key("esc")); cmd != nil || a.adding {
		t.Fatal("입력 중 Esc 는 취소만 해야 함")
	}
	if _, cmd := a.Update(key("esc")); cmd == nil {
		t.Fatal("목록에서 Esc 는 홈으로 가야 함")
	} else if _, ok := cmd().(goHomeMsg); !ok {
		t.Fatal("goHomeMsg 가 아님")
	}
}

func TestTodoPersist(t *testing.T) {
	p := todoSetup(t)
	a := todoNew()
	todoAdd(a, "보고서")
	todoAdd(a, "전화")
	todoKeys(a, "up", "space")
	b := todoNew() // 다시 실행
	if todoTexts(b) != "x보고서,-전화" {
		t.Fatalf("다시 불러온 결과: %s", todoTexts(b))
	}
	todoKeys(b, "d")
	if todoTexts(todoNew()) != "-전화" {
		t.Fatalf("삭제가 저장되지 않음")
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(p), "*.tmp")); len(m) != 0 {
		t.Fatalf("임시 파일이 남음: %v", m)
	}
}

func TestTodoLoadCorrupt(t *testing.T) {
	p := todoSetup(t)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("{깨진 파일"), 0o600)
	a := todoNew()
	if len(a.items) != 0 || a.msgTone != ToneWarn || !strings.Contains(a.msg, "깨져") {
		t.Fatalf("items=%v msg=%q", a.items, a.msg)
	}
	if b, err := os.ReadFile(p + ".bak"); err != nil || string(b) != "{깨진 파일" {
		t.Fatalf(".bak 보존 실패: %v %q", err, b)
	}
	assertFrame(t, "todo-corrupt", testCtx(56, 17), a.View(testCtx(56, 17)))
	todoAdd(a, "새 항목") // 새로 저장하면 정상 파일이 됨
	if todoTexts(todoNew()) != "-새 항목" {
		t.Fatal("깨진 뒤 새로 저장되지 않음")
	}
	if b, _ := os.ReadFile(p + ".bak"); string(b) != "{깨진 파일" {
		t.Fatal(".bak 이 바뀜")
	}
}

func TestTodoLoadEdgeCases(t *testing.T) {
	p := todoSetup(t)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(""), 0o600) // 빈 파일은 오류 없이 빈 목록
	if a := todoNew(); len(a.items) != 0 || a.msg != "" {
		t.Fatalf("빈 파일: %v %q", a.items, a.msg)
	}
	os.WriteFile(p, []byte(`{"items":[{"text":"  ","done":true},{"text":"a","done":true}]}`), 0o600)
	if got := todoTexts(todoNew()); got != "xa" {
		t.Fatalf("빈 글자 항목 걸러내기: %q", got)
	}
}

func TestTodoLoadUnreadable(t *testing.T) {
	p := todoSetup(t)
	os.MkdirAll(p, 0o755) // 파일 자리에 폴더가 있어 읽기 실패
	a := todoNew()
	if !a.noSave || a.msgTone != ToneWarn {
		t.Fatalf("noSave=%v msg=%q", a.noSave, a.msg)
	}
	todoAdd(a, "메모")
	if todoTexts(a) != "-메모" || a.msgTone != ToneWarn {
		t.Fatalf("목록은 계속 편집돼야 함: %s %q", todoTexts(a), a.msg)
	}
	if st, err := os.Stat(p); err != nil || !st.IsDir() {
		t.Fatal("읽지 못한 경로를 건드림")
	}
}

func TestTodoSaveFailure(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	os.WriteFile(blocker, []byte("x"), 0o600)
	old := todoPath
	todoPath = func() (string, error) { return filepath.Join(blocker, "sub", "todo.json"), nil } // 부모가 파일이라 폴더를 못 만듦
	t.Cleanup(func() { todoPath = old })
	a := todoNew()
	todoAdd(a, "저장 안 되는 항목")
	if a.msgTone != ToneWarn || !strings.Contains(a.msg, "저장하지 못함") {
		t.Fatalf("경고가 없음: %q", a.msg)
	}
	todoKeys(a, "space")
	todoAdd(a, "둘째")
	if todoTexts(a) != "x저장 안 되는 항목,-둘째" {
		t.Fatalf("저장 실패 뒤에도 편집돼야 함: %s", todoTexts(a))
	}
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		v := a.View(c)
		assertFrame(t, "todo-save-error", c, v)
		if !strings.Contains(plain(v), "저장하지 못함") {
			t.Errorf("%v: 경고 문구가 화면에 없음", sz)
		}
	}
	// 저장 위치가 정상으로 돌아오면 다시 저장되고 경고가 사라짐
	todoPath = func() (string, error) { return filepath.Join(dir, "ok", "todo.json"), nil }
	todoKeys(a, "space")
	if a.msg != "" {
		t.Fatalf("저장 성공 뒤에도 경고가 남음: %q", a.msg)
	}
}

func TestTodoNoConfigDir(t *testing.T) {
	old := todoPath
	todoPath = func() (string, error) { return "", fmt.Errorf("no dir") }
	t.Cleanup(func() { todoPath = old })
	a := todoNew()
	todoAdd(a, "x")
	if len(a.items) != 1 || a.msgTone != ToneWarn {
		t.Fatalf("items=%d msg=%q", len(a.items), a.msg)
	}
}

func TestTodoViews(t *testing.T) {
	long := strings.Repeat("아주 긴 할 일 문장입니다 ", 8)
	cases := map[string]func() *todoApp{
		"empty": todoNew,
		"few": func() *todoApp {
			a := todoNew()
			for _, s := range []string{"보고서 초안 마무리", "치과 예약 전화하기", "택배 반품 신청", "월요일 회의 자료 인쇄", "운동 30분"} {
				todoAdd(a, s)
			}
			todoKeys(a, "up", "up", "up", "up", "up", "space", "down", "down", "space") // 1, 3번째 완료 후 3번째 선택
			todoKeys(a, "up", "up")                                                     // 1번째 선택
			return a
		},
		"many-long": func() *todoApp {
			a := todoNew()
			for i := 0; i < 40; i++ {
				todoAdd(a, fmt.Sprintf("%d %s", i, long))
			}
			todoKeys(a, "space", "up", "up", "up", "up", "up", "up", "up", "up", "up", "space")
			return a
		},
		"adding": func() *todoApp {
			a := todoNew()
			todoAdd(a, "보고서")
			todoKeys(a, "a")
			todoType(a, "우유 사오기")
			return a
		},
	}
	// 입력이 입력줄 폭보다 길어도 박스 아래 줄이 흐트러지지 않는지
	cases["adding-long"] = func() *todoApp {
		a := todoNew()
		todoKeys(a, "a")
		todoType(a, strings.Repeat("우유 사오기 ", 10))
		return a
	}
	for name, mk := range cases {
		for _, sz := range testSizes {
			todoSetup(t) // 사례마다 빈 저장소에서 시작
			c := testCtx(sz[0], sz[1])
			a := mk()
			v := a.View(c)
			assertFrame(t, "todo-"+name, c, v)
			if h := strings.Count(v, "\n") + 1; h > c.H {
				t.Errorf("todo-%s %v: 화면 높이 %d 가 창 높이 %d 를 넘음", name, sz, h, c.H)
			}
		}
	}
	// 문구 확인
	todoSetup(t)
	c := testCtx(80, 24)
	if v := plain(todoNew().View(c)); !strings.Contains(v, "할 일이 없음") || !strings.Contains(v, "a 를 눌러 첫 항목을 추가함") {
		t.Errorf("빈 상태 문구가 없음:\n%s", v)
	}
	todoSetup(t)
	v := plain(cases["few"]().View(c))
	for _, want := range []string{"완료 2 / 5", "▸ [x] 보고서 초안 마무리", "[ ] 치과 예약 전화하기", "[x] 택배 반품 신청", "[Space] 완료", "[Esc] 홈으로"} {
		if !strings.Contains(v, want) {
			t.Errorf("기본 화면에 %q 가 없음:\n%s", want, v)
		}
	}
	todoSetup(t)
	v = plain(cases["adding"]().View(c))
	if !strings.Contains(v, "추가  > 우유") || !strings.Contains(v, "Enter = 저장    Esc = 취소") || strings.Contains(v, "▸") {
		t.Errorf("추가 입력 화면:\n%s", v)
	}
}

func TestTodoScrollFollowsSelection(t *testing.T) {
	todoSetup(t)
	a := todoNew()
	for i := 1; i <= 30; i++ {
		todoAdd(a, fmt.Sprintf("항목%02d", i))
	}
	c := testCtx(56, 17) // 목록 5줄
	v := plain(a.View(c))
	if !strings.Contains(v, "▸ [ ] 항목30") || strings.Contains(v, "항목01") || !strings.Contains(v, "보기 26-30") {
		t.Fatalf("맨 아래 선택:\n%s", v)
	}
	for i := 0; i < 29; i++ {
		todoKeys(a, "up")
	}
	v = plain(a.View(c))
	if !strings.Contains(v, "▸ [ ] 항목01") || strings.Contains(v, "항목30") || !strings.Contains(v, "보기 1-5") {
		t.Fatalf("맨 위 선택:\n%s", v)
	}
	for i := 0; i < 7; i++ {
		todoKeys(a, "down")
	}
	if v = plain(a.View(c)); !strings.Contains(v, "▸ [ ] 항목08") {
		t.Fatalf("선택 항목이 안 보임:\n%s", v)
	}
	// 삭제로 목록이 줄어도 범위를 벗어나지 않음
	for i := 0; i < 29; i++ {
		todoKeys(a, "d")
		assertFrame(t, "todo-scroll", c, a.View(c))
	}
	if len(a.items) != 1 || !strings.Contains(plain(a.View(c)), "항목") {
		t.Fatalf("items=%d", len(a.items))
	}
}

func TestTodoIgnoresOtherMessages(t *testing.T) {
	todoSetup(t)
	a := todoNew()
	a.Update(appTickMsg{Gen: 999, Tag: "x"})
	a.Update(tickMsg{})
	a.Update(tea.WindowSizeMsg{Width: 10, Height: 5})
	if len(a.items) != 0 {
		t.Fatal("상태가 바뀜")
	}
}

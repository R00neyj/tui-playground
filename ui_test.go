package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// 모든 앱 테스트가 함께 쓰는 도우미 (앱 담당자는 이 파일을 수정하지 말 것)

var fixedNow = time.Date(2026, 11, 5, 14, 32, 7, 0, time.Local)

func testCtx(w, h int) Ctx { return Ctx{W: w, H: h, Now: fixedNow} }

// 창 크기 후보: 넉넉한 창, 80x24, 큰 숫자 경계, 가장 작은 창
var testSizes = [][2]int{{120, 40}, {80, 24}, {80, 21}, {60, 20}, {56, 17}}

// assertFrame 은 앱이 그린 블록이 박스 모양을 지키는지 검사함:
// 테두리 줄은 모두 같은 폭이고, │ 로 시작한 줄은 │ 로 끝남 (내용이 넘쳐서 오른쪽 테두리가 밀리면 실패)
func assertFrame(t *testing.T, name string, c Ctx, view string) {
	t.Helper()
	in := c.In()
	boxLines := 0
	for i, raw := range strings.Split(view, "\n") {
		l := strings.TrimRight(ansi.Strip(raw), " ")
		if l == "" {
			continue
		}
		r := []rune(l)
		switch r[0] {
		case '╭', '├', '╰':
			if want := map[rune]rune{'╭': '╮', '├': '┤', '╰': '╯'}[r[0]]; r[len(r)-1] != want {
				t.Errorf("%s: %d번째 줄 끝 테두리 %q 이어야 함: %q", name, i, want, l)
			}
		case '│':
			if r[len(r)-1] != '│' {
				t.Errorf("%s: %d번째 줄이 │ 로 끝나지 않음(내용이 넘침?): %q", name, i, l)
			}
		default:
			continue // 박스 아래(바깥) 입력 줄 등
		}
		boxLines++
		if w := lipgloss.Width(l); w != in+2 {
			t.Errorf("%s: %d번째 줄 폭 %d, 기대 %d: %q", name, i, w, in+2, l)
		}
	}
	if boxLines < 4 {
		t.Errorf("%s: 박스 줄이 너무 적음(%d)", name, boxLines)
	}
}

func plain(s string) string { return ansi.Strip(s) }

func TestFrameComponentsAligned(t *testing.T) {
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		f := NewFrame(c).Blank()
		f.Row(Badge("예약됨", ToneOk), sg("   종료시각  ", colorTextMuted), sg("2026-11-05 15:10:00", colorText)).Blank()
		f.Digits("남은시간", "00:37:53", colorOk).Blank()
		f.Progress(0.37, colorOk).Rule()
		f.Menu("1", "새 타이머 설정", "기존 타이머는 자동 취소", false)
		f.MenuInline([2]string{"↑↓", "선택"}, [2]string{"1-9", "바로 열기"}, [2]string{"Enter", "실행"}, [2]string{"Esc", "종료"})
		f.Rule()
		f.Meter("메모리", 0.62, "9.9 / 16.0 GB", colorAccent)
		f.Meter("CPU", -1, "", colorAccent)
		f.Spark("추이", []float64{1, 5, 3, 8, 2, 7, 4}, 8, colorAccent)
		f.Tabs("기간", []string{"최근 5시간", "오늘", "이번 주"}, 1)
		f.Cells([]int{12, 8, 8, 8, 8}, []string{"claude-sonnet-5-5", "150,305", "41,246", "640,533", "10,000,402"}, colorText, false) // 좁은 창(안쪽 폭 50)에서도 들어가는 폭
		f.BarRow("11-05 오늘", 390, 612, "390K", true)
		f.Check("월요일 회의 자료 인쇄", false, true)
		f.Check("운동 30분", true, false)
		f.Rule().Message("메시지 확인용 한글 문장입니다", ToneWarn)
		f.Below(PromptLine("분  > ", "90"))
		assertFrame(t, "components", c, f.String())
	}
}

func TestBigRowsWidth(t *testing.T) {
	rows := bigRows("12:34:56.7")
	for i, r := range rows {
		if lipgloss.Width(r) != lipgloss.Width(rows[0]) {
			t.Errorf("row %d width %d != %d", i, lipgloss.Width(r), lipgloss.Width(rows[0]))
		}
	}
}

func TestTruncW(t *testing.T) {
	if got := truncW("가나다라마", 6); lipgloss.Width(got) > 6 || !strings.HasSuffix(got, "…") {
		t.Errorf("truncW=%q", got)
	}
	if got := truncW("abc", 6); got != "abc" {
		t.Errorf("truncW=%q", got)
	}
}

// DESIGN.md: 색 값은 토큰으로만 쓴다. theme.go 밖에서 lipgloss.Color(...) 로 색 번호를 직접 쓰면 실패함.
func TestNoRawColors(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	raw := regexp.MustCompile(`lipgloss\.Color\(`)
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || n == "theme.go" || strings.HasSuffix(n, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(n)
		if raw.Match(b) {
			t.Errorf("%s: lipgloss.Color(...) 를 직접 쓰지 말고 theme.go 의 토큰(colorText 등)을 쓸 것", n)
		}
	}
}

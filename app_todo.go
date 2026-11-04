package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 할 일 목록 (F-11). 변경할 때마다 사용자 설정 폴더의 todo.json 에 저장하고 다음 실행 때 불러옴.

var todoInfo = AppInfo{
	ID:   "todo",
	Name: "할 일",
	Desc: "간단한 할 일 목록",
	New:  newTodoApp,
}

// ---- 저장소 ----

// todoPath 는 저장 파일 경로. 테스트에서 임시 폴더로 바꿔 끼움.
var todoPath = func() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "TUI-playground", "todo.json"), nil
}

const todoMaxLen = 100 // 항목 하나의 최대 글자 수

type todoItem struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type todoFile struct {
	Items []todoItem `json:"items"`
}

// todoSave 는 임시 파일에 쓴 뒤 교체하는 방식으로 저장함
func todoSave(items []todoItem) error {
	p, err := todoPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(todoFile{Items: items}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "todo-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	if werr == nil {
		werr = tmp.Sync()
	}
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(name, p)
	}
	if werr != nil {
		os.Remove(name)
	}
	return werr
}

// todoLoadResult 는 파일을 읽은 결과. note 가 있으면 화면 메시지 줄에 보여줌.
type todoLoadResult struct {
	items  []todoItem
	note   string
	tone   Tone
	noSave bool // 읽지 못한 파일을 덮어쓰지 않으려고 저장을 막음
}

func todoLoad() todoLoadResult {
	p, err := todoPath()
	if err != nil {
		return todoLoadResult{note: "설정 폴더를 찾지 못함. 저장되지 않음", tone: ToneWarn}
	}
	data, err := os.ReadFile(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return todoLoadResult{}
	case err != nil:
		return todoLoadResult{note: "목록 파일을 읽지 못함. 저장하지 않음", tone: ToneWarn, noSave: true}
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return todoLoadResult{}
	}
	var tf todoFile
	if err := json.Unmarshal(data, &tf); err != nil {
		note := "저장 파일이 깨져 빈 목록으로 시작함 (원본은 .bak 로 보존)"
		if werr := os.WriteFile(p+".bak", data, 0o600); werr != nil {
			note = "저장 파일이 깨져 빈 목록으로 시작함 (원본 보존 실패)"
		}
		return todoLoadResult{note: note, tone: ToneWarn}
	}
	var items []todoItem
	for _, it := range tf.Items {
		if it.Text = strings.TrimSpace(it.Text); it.Text != "" {
			items = append(items, it)
		}
	}
	return todoLoadResult{items: items}
}

// ---- 앱 ----

type todoApp struct {
	items    []todoItem
	sel, top int
	adding   bool
	input    textinput.Model
	noSave   bool
	msg      string
	msgTone  Tone
}

func newTodoApp(env *Env) App {
	ti := textinput.New()
	ti.Prompt = "추가  > "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent)
	ti.CharLimit = todoMaxLen
	ti.Width = 36
	r := todoLoad()
	return &todoApp{items: r.items, input: ti, noSave: r.noSave, msg: r.note, msgTone: r.tone}
}

func (a *todoApp) Init() tea.Cmd { return nil }

func (a *todoApp) say(msg string, tone Tone) { a.msg, a.msgTone = msg, tone }

// save 는 변경할 때마다 부름. 실패해도 목록은 화면에서 계속 편집할 수 있음.
func (a *todoApp) save() {
	if a.noSave {
		a.say("불러오지 못해 저장하지 않음. 변경 내용은 이 화면에만 있음", ToneWarn)
		return
	}
	if err := todoSave(a.items); err != nil {
		a.say("저장하지 못함. 변경 내용은 이 화면에만 있음", ToneWarn)
		return
	}
	a.say("", ToneInfo)
}

func (a *todoApp) Update(msg tea.Msg) (App, tea.Cmd) {
	k, isKey := msg.(tea.KeyMsg)
	if a.adding {
		if isKey {
			switch k.String() {
			case "esc":
				a.stopAdding()
				return a, nil
			case "enter":
				if text := strings.TrimSpace(a.input.Value()); text != "" {
					a.items = append(a.items, todoItem{Text: text})
					a.sel = len(a.items) - 1
					a.save()
				}
				a.stopAdding()
				return a, nil
			}
		}
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg) // 키 입력과 커서 깜박임 등 입력창 내부 메시지
		return a, cmd
	}
	if !isKey {
		return a, nil
	}
	switch k.String() {
	case "esc":
		return a, goHome()
	case "a":
		a.adding = true
		a.input.SetValue("")
		return a, a.input.Focus()
	case "up", "k":
		a.sel = max(0, a.sel-1)
	case "down", "j":
		a.sel = max(0, min(len(a.items)-1, a.sel+1))
	case " ", "space":
		if len(a.items) > 0 {
			a.items[a.sel].Done = !a.items[a.sel].Done
			a.save()
		}
	case "d":
		if len(a.items) > 0 {
			a.items = append(a.items[:a.sel], a.items[a.sel+1:]...)
			a.sel = max(0, min(len(a.items)-1, a.sel))
			a.save()
		}
	}
	return a, nil
}

func (a *todoApp) stopAdding() {
	a.adding = false
	a.input.Blur()
	a.input.SetValue("")
}

// ---- 화면 ----

// todoFixedRows 는 목록 줄을 뺀 박스의 고정 줄 수(윗 테두리, 머리줄, 구분선, 제목, 빈 줄, 구분선, 키 안내, 구분선, 메시지, 아랫 테두리)
// todoBelowRows 는 추가 입력 중에 박스 아래에 붙는 줄 수. 입력 중이 아니어도 목록 높이를 똑같이 잡아서 화면이 출렁이지 않게 함.
const (
	todoFixedRows = 10
	todoBelowRows = 2
)

func (a *todoApp) View(c Ctx) string {
	n := len(a.items)
	h := n
	if c.H > 0 {
		h = max(1, c.H-todoFixedRows-todoBelowRows)
	}
	// 선택 항목이 보이도록 스크롤
	if a.sel < a.top {
		a.top = a.sel
	}
	if a.sel >= a.top+h {
		a.top = a.sel - h + 1
	}
	a.top = max(0, min(a.top, n-h))
	end := min(n, a.top+h)

	f := NewFrame(c)
	var right []seg
	if n > 0 {
		done := 0
		for _, it := range a.items {
			if it.Done {
				done++
			}
		}
		txt := fmt.Sprintf("완료 %d / %d", done, n)
		if n > h {
			txt += fmt.Sprintf("  보기 %d-%d", a.top+1, end)
		}
		right = []seg{sg(txt, colorTextMuted)}
	}
	f.RowLR([]seg{sgBold("할 일", colorText)}, right).Blank()
	if n == 0 {
		f.Row(sg("할 일이 없음", colorTextMuted))
		f.Row(sg("a 를 눌러 첫 항목을 추가함", colorTextMuted))
	}
	for i := a.top; i < end; i++ {
		f.Check(a.items[i].Text, a.items[i].Done, i == a.sel && !a.adding)
	}
	f.Rule()
	f.MenuInline([2]string{"a", "추가"}, [2]string{"Space", "완료"}, [2]string{"d", "삭제"}, [2]string{"↑↓", "이동"}, [2]string{"Esc", "홈으로"})
	f.Rule().Message(truncW(a.msg, f.In()-4), a.msgTone)
	if a.adding {
		f.Below(a.input.View(), fg("Enter = 저장    Esc = 취소", colorTextMuted))
	}
	return f.String()
}

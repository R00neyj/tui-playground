package main

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 점심 룰렛 (F-15). 메뉴는 사용자 설정 폴더의 TUI-playground/lunch.json 에 저장함.

var lunchInfo = AppInfo{
	ID:   "lunch",
	Name: "점심 룰렛",
	Desc: "오늘 점심 메뉴 뽑기",
	New:  newLunchApp,
}

var defaultLunchItems = []string{
	"김치찌개", "된장찌개", "제육볶음", "비빔밥", "순대국", "돈까스",
	"냉면", "칼국수", "라멘", "햄버거", "김밥", "쌀국수",
}

const (
	lunchMaxItems = 30
	lunchMaxRunes = 14
	lunchRows     = 5 // 바퀴가 보여주는 줄 수 (메뉴가 적으면 그만큼만 채우고 나머지는 빈 줄)
)

// 테스트에서 바꿔 끼울 수 있게 변수로 둠
var (
	lunchRand = rand.Intn
	lunchPath = func() string {
		dir, err := os.UserConfigDir()
		if err != nil || dir == "" {
			return ""
		}
		return filepath.Join(dir, "TUI-playground", "lunch.json")
	}
)

// loadLunchItems 는 파일이 없거나 깨졌으면 기본 메뉴를 돌려줌. 빈 목록으로 저장된 것은 그대로 존중함
func loadLunchItems() []string {
	p := lunchPath()
	if p == "" {
		return append([]string{}, defaultLunchItems...)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return append([]string{}, defaultLunchItems...)
	}
	var raw struct {
		Items *[]string `json:"items"`
	}
	if json.Unmarshal(data, &raw) != nil || raw.Items == nil {
		return append([]string{}, defaultLunchItems...)
	}
	var items []string
	for _, s := range *raw.Items {
		if s = strings.TrimSpace(s); s != "" && len(items) < lunchMaxItems {
			items = append(items, s)
		}
	}
	return items
}

// saveLunchItems 는 임시 파일에 쓴 뒤 교체함
func saveLunchItems(items []string) error {
	p := lunchPath()
	if p == "" {
		return os.ErrNotExist
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(struct {
		Items []string `json:"items"`
	}{items}, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

type lunchPhase int

const (
	lunchIdle lunchPhase = iota
	lunchSpinning
	lunchDone
)

type lunchApp struct {
	gen     int
	epoch   int // 돌릴 때마다 올림. 옛 틱이 새 회전에 섞이지 않게 함
	items   []string
	pos     int
	ph      lunchPhase
	step    int
	steps   int
	adding  bool
	input   textinput.Model
	msg     string
	msgTone Tone
}

func newLunchApp(env *Env) App {
	ti := textinput.New()
	ti.Prompt = "메뉴 이름  > "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent)
	ti.CharLimit = lunchMaxRunes
	ti.Width = lunchMaxRunes + 2
	return &lunchApp{gen: newGen(), items: loadLunchItems(), input: ti}
}

func (a *lunchApp) Init() tea.Cmd { return nil }

func (a *lunchApp) say(msg string, tone Tone) { a.msg, a.msgTone = msg, tone }

func (a *lunchApp) tag() string { return "lunch" + strconv.Itoa(a.epoch) }

// spinDelay 는 step 번째 칸으로 넘어가기까지의 간격. 끝에 가까울수록 느려짐
func spinDelay(step, steps int) time.Duration {
	r := float64(step) / float64(max(1, steps))
	return time.Duration(40+320*r*r*r) * time.Millisecond
}

func (a *lunchApp) start() tea.Cmd {
	n := len(a.items)
	if n == 0 {
		a.say("메뉴가 없음. a 로 추가함", ToneWarn)
		return nil
	}
	target := lunchRand(n)
	a.epoch++
	a.ph, a.step = lunchSpinning, 0
	a.steps = 2*n + (target-a.pos+n)%n // 최소 두 바퀴를 돈 뒤 target 에서 멈춤
	a.say("", ToneInfo)
	return tickCmd(spinDelay(0, a.steps), a.gen, a.tag())
}

func (a *lunchApp) finish() {
	a.ph = lunchDone
	a.say("오늘 점심 메뉴: "+a.items[a.pos], ToneOk)
}

func (a *lunchApp) Update(msg tea.Msg) (App, tea.Cmd) {
	switch m := msg.(type) {
	case appTickMsg:
		if m.Gen != a.gen || m.Tag != a.tag() || a.ph != lunchSpinning {
			return a, nil
		}
		a.pos = (a.pos + 1) % len(a.items)
		a.step++
		if a.step >= a.steps {
			a.finish()
			return a, nil
		}
		return a, tickCmd(spinDelay(a.step, a.steps), a.gen, a.tag())
	case tea.KeyMsg:
		if a.adding {
			return a.updateAdd(m)
		}
		return a.updateMain(m)
	}
	if a.adding { // 커서 깜박임 등 입력창 내부 메시지
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		return a, cmd
	}
	return a, nil
}

func (a *lunchApp) updateMain(k tea.KeyMsg) (App, tea.Cmd) {
	n := len(a.items)
	switch k.String() {
	case " ", "space", "enter":
		if a.ph == lunchSpinning { // 도는 중에 누르면 바로 결과로 건너뜀
			a.pos = (a.pos + a.steps - a.step) % n
			a.finish()
			return a, nil
		}
		return a, a.start()
	case "up", "k":
		if a.ph != lunchSpinning && n > 0 {
			a.pos = (a.pos + n - 1) % n
			a.ph = lunchIdle
			a.say("", ToneInfo)
		}
	case "down", "j":
		if a.ph != lunchSpinning && n > 0 {
			a.pos = (a.pos + 1) % n
			a.ph = lunchIdle
			a.say("", ToneInfo)
		}
	case "a", "A":
		if a.ph == lunchSpinning {
			return a, nil
		}
		if n >= lunchMaxItems {
			a.say("메뉴는 최대 "+strconv.Itoa(lunchMaxItems)+"개까지임", ToneWarn)
			return a, nil
		}
		a.adding = true
		a.input.SetValue("")
		a.say("", ToneInfo)
		return a, a.input.Focus()
	case "d", "D":
		if a.ph == lunchSpinning || n == 0 {
			return a, nil
		}
		name := a.items[a.pos]
		a.items = append(a.items[:a.pos], a.items[a.pos+1:]...)
		if a.pos >= len(a.items) {
			a.pos = 0
		}
		a.ph = lunchIdle
		a.persist(name + " 삭제함")
	case "esc", "q":
		return a, goHome()
	}
	return a, nil
}

func (a *lunchApp) updateAdd(k tea.KeyMsg) (App, tea.Cmd) {
	switch k.String() {
	case "esc":
		a.adding = false
		a.input.Blur()
		return a, nil
	case "enter":
		name := strings.TrimSpace(a.input.Value())
		if name == "" {
			a.adding = false
			a.input.Blur()
			return a, nil
		}
		for _, it := range a.items {
			if it == name {
				a.say(name+" 은(는) 이미 있음", ToneDanger)
				return a, nil
			}
		}
		a.items = append(a.items, name)
		a.pos = len(a.items) - 1
		a.ph = lunchIdle
		a.adding = false
		a.input.Blur()
		a.persist(name + " 추가함")
		return a, nil
	}
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(k)
	return a, cmd
}

// persist 는 저장하고 결과를 메시지로 알림. 저장에 실패해도 이번 실행에서는 바뀐 목록을 씀
func (a *lunchApp) persist(okMsg string) {
	if err := saveLunchItems(a.items); err != nil {
		a.say(okMsg+" (저장 못 함: 다음 실행에는 사라짐)", ToneWarn)
		return
	}
	a.say(okMsg, ToneOk)
}

// ---- 화면 ----

func (a *lunchApp) View(c Ctx) string {
	var badge seg
	switch a.ph {
	case lunchSpinning:
		badge = Badge("도는 중", ToneAccent)
	case lunchDone:
		badge = Badge("결정", ToneOk)
	default:
		badge = Badge("대기", ToneInfo)
	}

	// 입력 줄이 박스 아래에 2줄 붙으므로 입력 중에는 위쪽 빈 줄을 줄여 17줄 창에 맞춤
	f := NewFrame(c)
	if !a.adding {
		f.Blank()
	}
	f.Row(badge, sg("   메뉴 "+strconv.Itoa(len(a.items))+"개", colorTextMuted))
	if !a.adding {
		f.Blank()
	}
	a.wheel(f)
	f.Blank().Rule()

	switch {
	case a.adding:
		f.MenuInline([2]string{"Enter", "추가"}, [2]string{"Esc", "취소"})
	case a.ph == lunchSpinning:
		f.MenuInline([2]string{"Space", "바로 멈춤"}, [2]string{"Esc", "홈으로"})
	default:
		f.MenuInline([2]string{"Space", "돌리기"}, [2]string{"↑↓", "선택"}, [2]string{"a", "추가"}, [2]string{"d", "삭제"}, [2]string{"Esc", "홈으로"})
	}
	f.Rule().Message(a.msg, a.msgTone)
	if a.adding {
		f.Below("", a.input.View())
	}
	return f.String()
}

// wheel 은 현재 칸을 가운데 두고 앞뒤 메뉴를 보여줌 (Wheel). 항상 lunchRows 줄을 차지해서 화면이 흔들리지 않음
func (a *lunchApp) wheel(f *Frame) {
	n := len(a.items)
	rows := min(lunchRows, n)
	if n == 0 {
		f.Row(sg("등록된 메뉴 없음. a 로 추가함", colorTextMuted))
		for i := 1; i < lunchRows; i++ {
			f.Blank()
		}
		return
	}
	for i := 0; i < rows; i++ {
		idx := ((a.pos-rows/2+i)%n + n) % n
		if i == rows/2 {
			col := colorAccent
			if a.ph == lunchDone {
				col = colorOk
			}
			f.Row(sg("▸ ", colorAccent), sgBold(truncW(a.items[idx], f.In()-8), col))
			continue
		}
		f.Row(sg("  "+truncW(a.items[idx], f.In()-8), colorTextMuted))
	}
	for i := rows; i < lunchRows; i++ {
		f.Blank()
	}
}

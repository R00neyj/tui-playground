package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// 웹 브라우저(텍스트) (F-13). 주소를 열어 글 본문만 텍스트로 보여줌.
// 이미지·쿠키·로그인은 지원하지 않음. 자바스크립트는 직접 실행하지 않고, 글이 비는 페이지만 설치된 브라우저(Chrome/Edge)로 다시 그려서 읽음 (browser_render.go). 페이지에서 온 글자는 화면에 닿기 전에 모두 거름 (browser_parse.go).
// Esc 는 (불러오는 중이 아니면) 언제나 홈으로 감.

var browserInfo = AppInfo{
	ID:   "browser",
	Name: "웹 브라우저",
	Desc: "주소나 검색어로 글을 텍스트로 읽기",
	New:  func(env *Env) App { return newBrowserApp() },
}

type brMode int

const (
	brInput   brMode = iota // 주소 입력
	brRead                  // 읽기
	brLink                  // 링크 번호 입력
	brConfirm               // 다른 사이트로 이동 확인
)

const brMaxHist = 20

type brEntry struct {
	page   *brPage
	scroll int
}

// brDoneMsg 는 내려받기가 끝났을 때 돌아오는 메시지. Req 가 지금 요청과 다르면 무시함.
type brDoneMsg struct {
	Req    int
	Reload bool
	Asked  string
	Page   *brPage
	Err    error
}

type browserApp struct {
	gen   int
	mode  brMode
	input textinput.Model

	hist   []brEntry
	pos    int
	cur    *brPage
	scroll int

	msg     string
	msgTone Tone

	loading bool
	req     int
	cancel  context.CancelFunc
	started time.Time
	frame   int

	linkBuf string
	pending string

	jsMode int      // brJSAuto / brJSOn / brJSOff
	marks  []brMark // 빠른 이동 목록
	msel   int      // 주소 화면에서 고른 항목 (-1 = 고르지 않음)

	vw, vh int // 마지막으로 그린 본문 폭·높이
	layPg  *brPage
	layW   int
	lay    []brLine
}

func newBrowserApp() *browserApp {
	ti := textinput.New()
	ti.Prompt = "주소 > "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorTextMuted)
	ti.Placeholder = "주소 또는 검색어"
	ti.CharLimit = brMaxURL
	ti.Focus()
	return &browserApp{gen: newGen(), input: ti, pos: -1, vw: 46, vh: 6, marks: brLoadMarks(), msel: -1}
}

func (a *browserApp) Init() tea.Cmd { return textinput.Blink }

func (a *browserApp) say(msg string, tone Tone) { a.msg, a.msgTone = msg, tone }

// ---- 내려받기 ----

func (a *browserApp) cancelLoad() {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.loading = false
}

func (a *browserApp) startLoad(raw string, reload bool) tea.Cmd {
	a.cancelLoad()
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.loading, a.req, a.frame, a.started = true, newGen(), 0, time.Now()
	a.say("", ToneInfo)
	req, fetch, render, mode := a.req, browserFetch, browserRender, a.jsMode
	run := func() tea.Msg {
		p, err := brLoadPage(ctx, raw, mode, fetch, render)
		return brDoneMsg{Req: req, Reload: reload, Asked: raw, Page: p, Err: err}
	}
	return tea.Batch(run, tickCmd(150*time.Millisecond, a.gen, "load"))
}

func (a *browserApp) onDone(m brDoneMsg) {
	if m.Req != a.req || !a.loading {
		return
	}
	a.cancelLoad()
	if m.Err != nil {
		if errors.Is(m.Err, errBrCanceled) {
			a.say("불러오기를 취소함", ToneInfo)
		} else {
			a.say(brClip(brClean(m.Err.Error()), 200), ToneDanger)
		}
		return
	}
	if m.Page == nil {
		a.say("불러온 내용이 없음", ToneDanger)
		return
	}
	p := m.Page
	if m.Reload && a.pos >= 0 {
		a.hist[a.pos] = brEntry{page: p, scroll: a.scroll}
	} else {
		a.saveScroll()
		a.hist = append(a.hist[:a.pos+1], brEntry{page: p})
		if len(a.hist) > brMaxHist {
			a.hist = a.hist[len(a.hist)-brMaxHist:]
		}
		a.pos = len(a.hist) - 1
		a.scroll = 0
	}
	a.cur = p
	a.mode = brRead
	a.input.Blur()
	switch {
	case p.Truncated:
		a.say("본문이 2MB 를 넘어 앞부분만 읽음", ToneWarn)
	case p.Cut:
		a.say("문서가 너무 커서 일부만 정리함", ToneWarn)
	case !brSameSite(m.Asked, p.URL):
		a.say("다른 사이트로 이동됨: "+brClip(brSiteHost(p.URL), 60), ToneWarn)
	case p.Rendered:
		a.say("자바스크립트를 실행해 읽음 ("+brEngineName(brFindEngine())+")", ToneInfo)
	default:
		a.say("", ToneInfo)
	}
}

func (a *browserApp) saveScroll() {
	if a.pos >= 0 && a.pos < len(a.hist) {
		a.hist[a.pos].scroll = a.scroll
	}
}

// step 은 기록에서 앞(+1) 뒤(-1) 로 이동함
func (a *browserApp) step(d int) {
	t := a.pos + d
	if t < 0 || t >= len(a.hist) {
		if d < 0 {
			a.say("더 이전 기록이 없음", ToneWarn)
		} else {
			a.say("다음 기록이 없음", ToneWarn)
		}
		return
	}
	a.saveScroll()
	a.pos = t
	a.cur = a.hist[t].page
	a.scroll = a.hist[t].scroll
	a.say("", ToneInfo)
}

// ---- 키 처리 ----

func (a *browserApp) Update(msg tea.Msg) (App, tea.Cmd) {
	switch m := msg.(type) {
	case appTickMsg:
		if m.Gen != a.gen || m.Tag != "load" || !a.loading {
			return a, nil
		}
		a.frame++
		return a, tickCmd(150*time.Millisecond, a.gen, "load")
	case brDoneMsg:
		a.onDone(m)
		return a, nil
	case tea.KeyMsg:
		return a.onKey(m)
	}
	if a.mode == brInput && !a.loading { // 커서 깜박임 등 입력창 내부 메시지
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		return a, cmd
	}
	return a, nil
}

func (a *browserApp) onKey(k tea.KeyMsg) (App, tea.Cmd) {
	s := k.String()
	if a.loading {
		if s == "esc" { // 요청을 취소하고 홈으로 (Esc 는 언제나 홈으로 감)
			a.cancelLoad()
			a.say("불러오기를 취소함", ToneInfo)
			return a, goHome()
		}
		return a, nil
	}
	if s == "esc" {
		return a, goHome()
	}
	switch a.mode {
	case brInput:
		return a.keyInput(k)
	case brLink:
		return a.keyLink(s)
	case brConfirm:
		a.mode = brRead
		if s == "y" {
			return a, a.startLoad(a.pending, false)
		}
		a.say("이동을 취소함", ToneInfo)
		return a, nil
	}
	return a.keyRead(s)
}

func (a *browserApp) keyInput(k tea.KeyMsg) (App, tea.Cmd) {
	key := k.String()
	empty := strings.TrimSpace(a.input.Value()) == ""
	if empty && len(a.marks) > 0 { // 입력창이 비어 있을 때만 위아래 키로 빠른 이동 목록을 고름
		n := len(a.marks)
		switch key {
		case "up":
			a.msel = (a.msel + n - 1) % n
			if a.msel < 0 {
				a.msel = n - 1
			}
			return a, nil
		case "down":
			a.msel = (a.msel + 1) % n
			return a, nil
		case "ctrl+d":
			a.deleteMark()
			return a, nil
		}
	}
	if key == "enter" {
		v := strings.TrimSpace(a.input.Value())
		if v == "" {
			switch {
			case a.msel >= 0 && a.msel < len(a.marks):
				return a, a.startLoad(a.marks[a.msel].URL, false)
			case a.cur != nil:
				a.mode = brRead
				a.input.Blur()
				a.say("", ToneInfo)
			default:
				a.say("주소나 검색어를 입력하거나 ↑↓ 로 고를 것", ToneWarn)
			}
			return a, nil
		}
		raw, _ := brResolveInput(v)
		norm, err := brNormalizeURL(raw)
		if err != nil {
			a.say(err.Error(), ToneDanger)
			return a, nil
		}
		return a, a.startLoad(norm, false)
	}
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(k)
	if strings.TrimSpace(a.input.Value()) != "" {
		a.msel = -1
	}
	return a, cmd
}

func (a *browserApp) keyLink(s string) (App, tea.Cmd) {
	switch {
	case s == "enter":
		return a, a.follow()
	case s == "backspace":
		if a.linkBuf = a.linkBuf[:max(0, len(a.linkBuf)-1)]; a.linkBuf == "" {
			a.mode = brRead
		}
	case len(s) == 1 && s[0] >= '0' && s[0] <= '9':
		if len(a.linkBuf) < 4 {
			a.linkBuf += s
		}
	default:
		a.mode = brRead
	}
	return a, nil
}

// follow 는 입력한 번호의 링크로 감. 다른 사이트면 확인을 한 번 거침.
func (a *browserApp) follow() tea.Cmd {
	n, _ := strconv.Atoi(a.linkBuf)
	a.mode = brRead
	if a.cur == nil || n < 1 || n > len(a.cur.Links) {
		a.say(fmt.Sprintf("링크 %s 번이 없음", a.linkBuf), ToneDanger)
		return nil
	}
	href := a.cur.Links[n-1]
	if brSameSite(a.cur.URL, href) {
		return a.startLoad(href, false)
	}
	a.pending, a.mode = href, brConfirm
	return nil
}

func (a *browserApp) keyRead(s string) (App, tea.Cmd) {
	page := max(1, a.vh-1)
	switch s {
	case "j", "down":
		a.scrollBy(1)
	case "k", "up":
		a.scrollBy(-1)
	case " ", "pgdown", "ctrl+f":
		a.scrollBy(page)
	case "pgup", "ctrl+b":
		a.scrollBy(-page)
	case "g", "home":
		a.scroll = 0
	case "G", "end":
		a.scroll = len(a.lines())
		a.scrollBy(0)
	case "o", "/":
		a.mode = brInput
		a.input.SetValue("")
		a.say("", ToneInfo)
		return a, a.input.Focus()
	case "f":
		a.mode, a.linkBuf = brLink, ""
		a.say("", ToneInfo)
	case "h", "backspace", "left":
		a.step(-1)
	case "l", "right":
		a.step(1)
	case "r":
		if a.cur != nil {
			return a, a.startLoad(a.cur.URL, true)
		}
	case "m": // 자바스크립트 렌더링 방식: 자동 → 항상 → 끔
		a.jsMode = (a.jsMode + 1) % len(brJSNames)
		a.say("자바스크립트 렌더링: "+brJSNames[a.jsMode]+" (r 로 다시 읽으면 적용)", ToneInfo)
	case "b":
		a.addMark()
	default:
		if len(s) == 1 && s[0] >= '0' && s[0] <= '9' {
			a.mode, a.linkBuf = brLink, s
			a.say("", ToneInfo)
		}
	}
	return a, nil
}

// ---- 스크롤 ----

// lines 는 현재 문서를 마지막으로 그린 폭에 맞춰 펼친 줄들 (같은 문서·폭이면 캐시를 씀)
func (a *browserApp) lines() []brLine {
	if a.cur == nil {
		return nil
	}
	if a.layPg != a.cur || a.layW != a.vw {
		a.lay, a.layPg, a.layW = brLayoutPage(a.cur, a.vw), a.cur, a.vw
	}
	return a.lay
}

func (a *browserApp) scrollBy(n int) {
	top := max(0, len(a.lines())-a.vh)
	a.scroll = min(top, max(0, a.scroll+n))
}

// ---- 화면 ----

func (a *browserApp) View(c Ctx) string {
	f := NewFrame(c)
	in := f.In()
	tw := in - 4
	below := 0
	if a.mode == brInput {
		below = 1
	}
	a.vw, a.vh = tw, max(1, c.H-10-below)
	lines := a.lines()
	a.scrollBy(0)

	// 제목 줄, 주소 줄
	title := sgBold(appNameBrowser, colorAccent)
	urlLeft, pos := "주소를 입력하면 글을 텍스트로 보여줌", ""
	if a.cur != nil {
		title = sgBold(brTrunc(a.cur.Title, tw), colorText)
		urlLeft = a.cur.URL
		bottom := min(len(lines), a.scroll+a.vh)
		pct := 100
		if len(lines) > 0 {
			pct = bottom * 100 / len(lines)
		}
		pos = fmt.Sprintf("%d/%d  %d%%", bottom, len(lines), pct)
	}
	urlCol := colorTextMuted
	if a.mode == brConfirm {
		urlLeft, urlCol = "→ "+a.pending, colorWarn
	}
	f.Row(title)
	f.RowLR([]seg{sg(brTrunc(brClean(urlLeft), max(4, tw-lipgloss.Width(pos)-2)), urlCol)}, []seg{sg(pos, colorTextMuted)})
	f.Rule()

	// 본문
	if a.mode == brInput || a.cur == nil {
		rows := a.inputRows(tw)
		for i := 0; i < a.vh; i++ {
			if i < len(rows) {
				f.Row(rows[i]...)
			} else {
				f.Blank()
			}
		}
	} else {
		for i := 0; i < a.vh; i++ {
			if a.scroll+i < len(lines) {
				f.Row(brFitLine(lines[a.scroll+i], tw)...)
			} else {
				f.Blank()
			}
		}
	}
	f.Rule()

	// 메시지 줄, 키 안내
	msg, tone := a.statusLine(c, tw)
	f.Message(brTrunc(brClean(msg), tw), tone)
	switch {
	case a.loading:
		f.MenuInline([2]string{"Esc", "취소하고 홈으로"})
	case a.mode == brInput:
		f.MenuInline([2]string{"Enter", "열기"}, [2]string{"↑↓", "고르기"}, [2]string{"Esc", "홈으로"})
	case a.mode == brLink:
		f.MenuInline([2]string{"Enter", "이동"}, [2]string{"Backspace", "지움"}, [2]string{"Esc", "홈으로"})
	case a.mode == brConfirm:
		f.MenuInline([2]string{"y", "이동"}, [2]string{"n", "취소"}, [2]string{"Esc", "홈으로"})
	default:
		// 폭이 모자라면 두 번째 항목부터 빠지므로 덜 중요한 것을 앞쪽에 둠
		f.MenuInline([2]string{"↑↓", "스크롤"}, [2]string{"r", "다시"}, [2]string{"m", "JS"}, [2]string{"b", "저장"}, [2]string{"h/l", "뒤/앞"}, [2]string{"o", "주소"}, [2]string{"번호", "링크"}, [2]string{"Esc", "홈으로"})
	}
	if below > 0 {
		// 입력창의 가로 스크롤 위치는 폭이 바뀔 때 다시 계산해야 함
		a.input.Width = max(8, in-4-lipgloss.Width(a.input.Prompt)-2)
		a.input.SetCursor(a.input.Position())
		f.Below(ansi.Truncate(a.input.View(), in-2, ""))
	}
	return f.String()
}

const appNameBrowser = "웹 브라우저"

// statusLine 은 메시지 줄에 보일 글: 불러오는 중, 링크 미리보기, 이동 확인, 그 밖의 알림 순
func (a *browserApp) statusLine(c Ctx, tw int) (string, Tone) {
	switch {
	case a.loading:
		el := int(c.Now.Sub(a.started).Seconds())
		if el < 0 {
			el = 0
		}
		return fmt.Sprintf("불러오는 중 %c  %d초", "|/-\\"[a.frame%4], el), ToneAccent
	case a.mode == brLink:
		if a.linkBuf == "" {
			return "링크 번호를 입력하고 Enter", ToneInfo
		}
		n, _ := strconv.Atoi(a.linkBuf)
		if a.cur == nil || n < 1 || n > len(a.cur.Links) {
			return fmt.Sprintf("링크 %s 번이 없음", a.linkBuf), ToneDanger
		}
		href := a.cur.Links[n-1]
		if !brSameSite(a.cur.URL, href) {
			return fmt.Sprintf("[%s] 다른 사이트 → %s", a.linkBuf, href), ToneWarn
		}
		return fmt.Sprintf("[%s] → %s", a.linkBuf, href), ToneInfo
	case a.mode == brConfirm:
		return "다른 사이트로 이동: " + brSiteHost(a.pending), ToneWarn
	}
	return a.msg, a.msgTone
}

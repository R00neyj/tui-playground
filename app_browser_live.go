package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 웹 브라우저의 라이브 모드 (F-13): 설치된 Chrome/Edge 로 페이지를 실제처럼 그리고, 방향키로 가상 커서를 움직여 Enter 로 클릭함.

// brAct 는 라이브 세션에 시킬 동작 하나
type brAct struct {
	kind string // click text key scroll top bottom select back fwd reload resize snap
	x, y float64
	dy   float64
	text string
	key  string
	idx  int
	cols int
	rows int
}

// brLiveMsg 는 라이브 불러오기나 동작이 끝났을 때 돌아오는 메시지
type brLiveMsg struct {
	Req   int
	Act   bool
	Moved bool // 스크롤·클릭처럼 페이지를 바꿀 수 있는 동작이 들어 있음 (잠시 뒤 한 번 더 읽음)
	Live  brLive
	Snap  *brSnap
	Page  *brPage // 라이브가 실패해 리더 모드로 읽은 결과
	Note  string
	Asked string
	Err   error
}

type brLiveState struct {
	live       brLive
	on         bool // 지금 화면이 라이브 모드임
	readerOnly bool // 사용자가 리더 모드를 골랐음 (v)
	snap       *brSnap
	scr        *brScreen
	prow, pcol int
	roff       int // 계산된 줄이 화면보다 많을 때 화면 맨 윗줄이 가리키는 줄
	edit       bool
	queue      []brAct
	busy       bool
	cols, rows int // 세션 화면 크기
	lastMove   time.Time
	lastDir    int
	refreshing bool
	settle     int // 늦게 그려지는 내용을 따라가려고 남은 다시 읽기 횟수
	fitted     bool // 줄이 화면보다 많아서 브라우저 화면 높이를 줄인 상태
	fitTries   int
	noFit      bool
	streak     int
}

func (a *browserApp) wantLive() bool { return !a.ls.readerOnly && brFindEngine() != "" }

func (a *browserApp) closeLive() {
	if a.ls.live != nil {
		a.ls.live.Close()
	}
	a.ls = brLiveState{readerOnly: a.ls.readerOnly}
}

// Close 는 홈으로 돌아가거나 프로그램을 끝낼 때 호스트가 부름 (브라우저 프로세스를 정리함)
func (a *browserApp) Close() {
	a.cancelLoad()
	a.closeLive()
}

// ---- 불러오기 ----

func (a *browserApp) startLive(raw string) tea.Cmd {
	a.cancelLoad()
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.loading, a.req, a.frame, a.started = true, newGen(), 0, time.Now()
	a.say("", ToneInfo)
	req, live, cols, rows, exe := a.req, a.ls.live, a.vw, a.vh+1, brFindEngine()
	fetch, render, mode := browserFetch, browserRender, a.jsMode
	regrow := a.ls.fitted && live != nil
	if regrow {
		a.ls.fitted, a.ls.rows = false, rows
	}
	a.ls.fitTries = 0
	if live == nil {
		a.ls.cols, a.ls.rows = cols, rows
	}
	run := func() tea.Msg {
		norm, err := brNormalizeURL(raw)
		if err != nil {
			return brLiveMsg{Req: req, Live: live, Err: err}
		}
		norm = brSiteRewrite(norm)
		opened := false
		if live == nil {
			live, err = brOpenLive(ctx, exe, cols, rows)
			opened = err == nil
		}
		var snap *brSnap
		if err == nil && regrow {
			err = live.Resize(ctx, cols, rows)
		}
		if err == nil {
			err = live.Navigate(ctx, norm)
		}
		if err == nil {
			snap, err = live.Snapshot(ctx)
		}
		if err == nil {
			return brLiveMsg{Req: req, Live: live, Snap: snap, Asked: raw}
		}
		if ctx.Err() != nil || errors.Is(err, errBrCanceled) {
			if opened && live != nil {
				live.Close()
				live = nil
			}
			return brLiveMsg{Req: req, Live: live, Err: errBrCanceled}
		}
		p, perr := brLoadPage(ctx, raw, mode, fetch, render)
		if ctx.Err() != nil && opened {
			live.Close()
			return brLiveMsg{Req: req, Err: errBrCanceled}
		}
		if perr != nil {
			return brLiveMsg{Req: req, Live: live, Err: perr}
		}
		return brLiveMsg{Req: req, Live: live, Page: p, Asked: raw, Note: "라이브 모드 실패, 리더 모드로 읽음: " + brClip(brClean(err.Error()), 80)}
	}
	return tea.Batch(run, tickCmd(150*time.Millisecond, a.gen, "load"))
}

func (a *browserApp) onLiveMsg(m brLiveMsg) tea.Cmd {
	if m.Live != nil && a.ls.live == nil {
		a.ls.live = m.Live
	} else if m.Live != nil && m.Live != a.ls.live {
		m.Live.Close()
	}
	if m.Act {
		return a.onActDone(m)
	}
	if m.Req != a.req || !a.loading {
		return nil
	}
	if m.Snap == nil {
		if errors.Is(m.Err, errBrLiveGone) {
			a.closeLive()
		}
		a.ls.on = false
		a.onDone(brDoneMsg{Req: m.Req, Asked: m.Asked, Page: m.Page, Err: m.Err})
		if m.Page != nil && m.Note != "" {
			a.say(m.Note, ToneWarn)
		}
		return nil
	}
	a.cancelLoad()
	a.ls.on = true
	a.ls.edit = false
	a.ls.queue = nil
	a.applySnap(m.Snap)
	a.mode = brRead
	a.input.Blur()
	if r, c, ok := a.ls.scr.nextAnchor(-1, 0, 1); ok {
		a.ls.prow, a.ls.pcol = r, c
	} else {
		a.ls.prow, a.ls.pcol = 0, 0
	}
	a.showRow(a.ls.prow)
	fit := a.refit()
	if brSameSite(m.Asked, m.Snap.URL) {
		a.say("", ToneInfo)
	} else {
		a.say("다른 사이트로 이동됨: "+brClip(brSiteHost(m.Snap.URL), 60), ToneWarn)
	}
	a.ls.settle, a.ls.refreshing = brSettleTries-1, true
	return tea.Batch(fit, tickCmd(1500*time.Millisecond, a.gen, "lrefresh"))
}

func (a *browserApp) applySnap(s *brSnap) {
	prevSY, prevURL := 0.0, ""
	if a.ls.snap != nil {
		prevSY, prevURL = a.ls.snap.SY, a.ls.snap.URL
	}
	a.ls.snap = s
	a.ls.scr = brLayoutSnap(s, max(1, a.ls.cols))
	title := s.Title
	if title == "" {
		title = brSiteHost(s.URL)
	}
	a.cur = &brPage{URL: s.URL, Title: title}
	a.ls.pcol = max(0, min(a.ls.pcol, max(0, a.ls.cols-1)))
	if s.SY != prevSY || s.URL != prevURL {
		a.ls.roff = 0
	}
	a.ls.roff = max(0, min(a.ls.roff, len(a.ls.scr.rows)-a.vh))
	a.ls.prow = max(a.ls.roff, min(a.ls.prow, a.ls.roff+max(0, a.vh-1)))
}

// ---- 동작 대기열 ----

func (a *browserApp) act(x brAct) tea.Cmd {
	a.ls.queue = append(a.ls.queue, x)
	return a.pump()
}

func brMergeActs(in []brAct) []brAct {
	var out []brAct
	for _, x := range in {
		if n := len(out); n > 0 {
			last := &out[n-1]
			switch {
			case x.kind == "text" && last.kind == "text":
				last.text += x.text
				continue
			case x.kind == "scroll" && last.kind == "scroll":
				last.dy += x.dy
				continue
			case x.kind == "snap" && last.kind == "snap":
				continue
			}
		}
		out = append(out, x)
	}
	return out
}

func brDoAct(ctx context.Context, l brLive, x brAct) error {
	switch x.kind {
	case "click":
		return l.Click(ctx, x.x, x.y)
	case "text":
		return l.InsertText(ctx, x.text)
	case "key":
		return l.Key(ctx, x.key)
	case "scroll":
		return l.Scroll(ctx, x.x, x.y, x.dy)
	case "top":
		return l.ScrollTo(ctx, true)
	case "bottom":
		return l.ScrollTo(ctx, false)
	case "select":
		return l.SelectNext(ctx, x.idx)
	case "back":
		return l.History(ctx, true)
	case "fwd":
		return l.History(ctx, false)
	case "reload":
		return l.Reload(ctx)
	case "resize":
		return l.Resize(ctx, x.cols, x.rows)
	}
	return nil
}

func (a *browserApp) pump() tea.Cmd {
	if a.ls.busy || len(a.ls.queue) == 0 || a.ls.live == nil {
		return nil
	}
	acts := brMergeActs(a.ls.queue)
	moved := false
	for _, x := range acts {
		moved = moved || (x.kind != "snap" && x.kind != "resize")
	}
	a.ls.queue, a.ls.busy = nil, true
	live, req := a.ls.live, a.req
	run := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), brActionBudget)
		defer cancel()
		var err error
		for _, x := range acts {
			if err = brDoAct(ctx, live, x); err != nil {
				break
			}
		}
		var snap *brSnap
		if err == nil {
			snap, err = live.Snapshot(ctx)
		}
		return brLiveMsg{Req: req, Act: true, Moved: moved, Snap: snap, Err: err}
	}
	return tea.Batch(run, tickCmd(150*time.Millisecond, a.gen, "busy"))
}

func (a *browserApp) onActDone(m brLiveMsg) tea.Cmd {
	a.ls.busy = false
	if m.Err != nil {
		a.ls.queue = nil
		if errors.Is(m.Err, errBrLiveGone) {
			a.closeLive()
			a.say("브라우저가 종료됨. o 로 주소를 다시 열 것", ToneDanger)
		} else {
			a.say(brClip(brClean(m.Err.Error()), 120), ToneDanger)
		}
		return nil
	}
	if m.Req != a.req || m.Snap == nil || !a.ls.on {
		return a.pump()
	}
	prev := ""
	if a.ls.snap != nil {
		prev = a.ls.snap.URL
	}
	if m.Moved {
		a.ls.fitTries = 0
	}
	a.applySnap(m.Snap)
	fit := a.refit()
	if m.Snap.URL != prev {
		a.ls.edit = false
		if !brSameSite(prev, m.Snap.URL) {
			a.say("다른 사이트로 이동됨: "+brClip(brSiteHost(m.Snap.URL), 60), ToneWarn)
		}
	}
	if m.Moved {
		a.ls.settle = brSettleTries
	}
	if a.ls.settle > 0 && !a.ls.refreshing {
		a.ls.refreshing = true
		a.ls.settle--
		return tea.Batch(fit, a.pump(), tickCmd(900*time.Millisecond, a.gen, "lrefresh"))
	}
	return tea.Batch(fit, a.pump())
}

// refit 은 칸 줄 수가 화면 줄 수에 가깝도록 브라우저 화면 높이를 줄이거나(넘칠 때) 늘림(아래에 내용이 더 있는데 줄이 80% 미만일 때)
func (a *browserApp) refit() tea.Cmd {
	l := &a.ls
	n := len(l.scr.rows)
	if l.noFit || l.fitTries >= 3 || l.snap == nil || n == 0 {
		return nil
	}
	more := l.snap.SY+l.snap.VH < l.snap.DH-brCellH
	var nr int
	switch {
	case n > a.vh && l.rows > 6:
		nr = max(6, l.rows*a.vh/n)
	case more && n*5 < a.vh*4:
		nr = min(brFitMaxRows(a.vh), l.rows*a.vh/n)
	default:
		return nil
	}
	if nr == l.rows {
		return nil
	}
	l.rows, l.fitted = nr, true
	l.fitTries++
	return a.act(brAct{kind: "resize", cols: l.cols, rows: nr})
}

func brFitMaxRows(vh int) int { return min(200, (vh+1)*3) }

// resizeIfNeeded 는 창 크기가 바뀌었을 때 세션 화면 크기를 맞춤
func (a *browserApp) resizeIfNeeded() tea.Cmd {
	if !a.ls.on || a.ls.live == nil || a.loading || a.ls.cols <= 0 {
		return nil
	}
	if a.vw == a.ls.cols && a.vh <= a.ls.rows && (a.ls.fitted || a.vh >= a.ls.rows-4) {
		return nil
	}
	a.ls.cols, a.ls.rows, a.ls.fitted, a.ls.fitTries = a.vw, a.vh, false, 0
	return a.act(brAct{kind: "resize", cols: a.vw, rows: a.vh})
}

// ---- 키 ----

// scrollAct 는 커서가 있는 자리에서 휠을 돌리는 동작 (옆 패널이 아니라 커서 아래 영역이 스크롤됨)
func (a *browserApp) scrollAct(dy float64) brAct {
	x, y := 40.0, 40.0
	if a.ls.scr != nil {
		x, y = a.ls.scr.cellPx(a.ls.prow, a.ls.pcol)
	}
	return brAct{kind: "scroll", x: x, y: y, dy: dy}
}

func (a *browserApp) pageDy() float64 {
	if s := a.ls.snap; s != nil && s.VH > 0 {
		return s.VH * 0.9
	}
	return float64(a.vh) * brCellH * 0.9
}

func (a *browserApp) keyLive(k tea.KeyMsg) (App, tea.Cmd) {
	s := k.String()
	if a.ls.edit {
		return a.keyLiveEdit(k)
	}
	a.say("", ToneInfo)
	switch s {
	case "up":
		return a, a.moveCursor(-1, 0)
	case "down":
		return a, a.moveCursor(1, 0)
	case "left":
		return a, a.moveCursor(0, -1)
	case "right":
		return a, a.moveCursor(0, 1)
	case "enter":
		return a, a.clickCursor()
	case "tab", "shift+tab":
		dir := 1
		if s == "shift+tab" {
			dir = -1
		}
		if r, c, ok := a.ls.scr.nextAnchor(a.ls.prow, a.ls.pcol, dir); ok {
			a.ls.prow, a.ls.pcol = r, min(c, a.vw-1)
			a.showRow(r)
		} else {
			a.say("누를 수 있는 곳이 없음", ToneInfo)
		}
	case "j":
		return a, a.act(a.scrollAct(brCellH * 2))
	case "k":
		return a, a.act(a.scrollAct(-brCellH * 2))
	case " ", "pgdown", "ctrl+f":
		return a, a.pageMove(1)
	case "pgup", "ctrl+b":
		return a, a.pageMove(-1)
	case "g", "home":
		return a, a.act(brAct{kind: "top"})
	case "G", "end":
		return a, a.act(brAct{kind: "bottom"})
	case "o", "/", "ctrl+l":
		a.mode = brInput
		a.input.SetValue("")
		a.say("", ToneInfo)
		return a, a.input.Focus()
	case "h", "backspace", "alt+left":
		return a, a.act(brAct{kind: "back"})
	case "l", "alt+right":
		return a, a.act(brAct{kind: "fwd"})
	case "r":
		return a, a.act(brAct{kind: "reload"})
	case "b":
		a.addMark()
	case "v":
		return a.toReader()
	case "m":
		a.say("라이브 모드에서는 항상 자바스크립트를 실행함 (v 로 리더 모드 전환)", ToneInfo)
	}
	return a, nil
}

// toReader 는 지금 페이지를 라이브 모드를 끄고 글만 뽑는 리더 모드로 다시 읽음
func (a *browserApp) toReader() (App, tea.Cmd) {
	if a.cur == nil {
		return a, nil
	}
	raw := a.cur.URL
	a.closeLive()
	a.ls.readerOnly = true
	a.say("리더 모드로 읽음 (v 를 다시 누르면 라이브 모드)", ToneInfo)
	return a, a.startLoad(raw, false)
}

func (a *browserApp) moveCursor(dr, dc int) tea.Cmd {
	l := &a.ls
	now := time.Now()
	dir := dr*3 + dc
	if dir == l.lastDir && now.Sub(l.lastMove) < 170*time.Millisecond {
		l.streak++
	} else {
		l.streak = 0
	}
	l.lastDir, l.lastMove = dir, now
	stepC := 1 + min(l.streak/3, 6)
	stepR := 1 + min(l.streak/8, 2)
	nr, nc := l.prow+dr*stepR, l.pcol+dc*stepC
	var cmd tea.Cmd
	switch {
	case nr < l.roff:
		if l.roff > 0 {
			l.roff = max(0, nr)
		} else {
			cmd = a.act(a.scrollAct(-brCellH * 3))
			nr = 0
		}
	case nr >= l.roff+a.vh:
		if l.roff+a.vh < len(l.scr.rows) {
			l.roff = min(nr-a.vh+1, len(l.scr.rows)-a.vh)
		} else {
			cmd = a.act(a.scrollAct(brCellH * 3))
			nr = l.roff + max(0, a.vh-1)
		}
	}
	l.prow = nr
	l.pcol = max(0, min(nc, max(0, a.vw-1)))
	return cmd
}

// itemAt 은 커서 칸에 놓인 항목 (없으면 false)
func (sc *brScreen) itemAt(row, col int) (brItem, bool) {
	if row < 0 || row >= len(sc.rows) {
		return brItem{}, false
	}
	for _, it := range sc.rows[row] {
		if col >= it.col && col < it.col+it.width() {
			return it, true
		}
	}
	return brItem{}, false
}

func (a *browserApp) clickCursor() tea.Cmd {
	l := &a.ls
	x, y := l.scr.cellPx(l.prow, l.pcol)
	if it, ok := l.scr.itemAt(l.prow, l.pcol); ok && it.elem >= 0 && it.elem < len(l.snap.Elems) {
		e := l.snap.Elems[it.elem]
		switch e.Kind {
		case "input":
			l.edit = true
			a.say("입력 중: 글자를 치고 Enter 로 제출, Esc 로 입력 끝", ToneAccent)
			return a.act(brAct{kind: "click", x: e.X + e.W/2, y: e.Y + e.H/2})
		case "select":
			return a.act(brAct{kind: "select", idx: it.elem})
		case "other":
			a.say("파일·색·슬라이더 입력은 지원하지 않음", ToneWarn)
			return nil
		case "check", "button":
			if e.Widget == 1 {
				x, y = e.X+e.W/2, e.Y+e.H/2
			}
		}
	}
	return a.act(brAct{kind: "click", x: x, y: y})
}

func (a *browserApp) keyLiveEdit(k tea.KeyMsg) (App, tea.Cmd) {
	s := k.String()
	switch s {
	case "tab", "shift+tab":
		a.ls.edit = false
		a.say("", ToneInfo)
		return a, nil
	case "enter", "backspace", "delete", "left", "right", "up", "down", "home", "end":
		return a, a.act(brAct{kind: "key", key: s})
	}
	if k.Type == tea.KeyRunes || k.Type == tea.KeySpace {
		if t := brClean(string(k.Runes)); t != "" {
			return a, a.act(brAct{kind: "text", text: t})
		}
	}
	return a, nil
}

// ---- 화면 ----

type brCell struct {
	s    string
	w    int
	fg   lipgloss.Color
	bg   lipgloss.Color
	bold bool
	elem int
}

func brItemStyle(k brItemKind, bold bool) (lipgloss.Color, bool) {
	switch k {
	case biHead:
		return colorAccent, true
	case biLink, biBracket:
		return colorAccent, bold
	case biMuted, biHint:
		return colorTextMuted, false
	}
	return colorText, bold
}

// liveRow 는 칸 격자 한 줄을 색 입힌 조각들로 만듦. 커서 칸과 커서 아래 요소는 눈에 띄게 칠함.
func (a *browserApp) liveRow(r, tw int, hover int) []seg {
	r += a.ls.roff
	var cells []brCell
	var items []brItem
	if r < len(a.ls.scr.rows) {
		items = a.ls.scr.rows[r]
	}
	for _, it := range items {
		for len(cells) < it.col {
			cells = append(cells, brCell{s: " ", w: 1, fg: colorText, elem: -1})
		}
		if len(cells) > it.col {
			continue
		}
		if it.kind == biPic && a.ls.snap != nil && a.ls.snap.Shot != nil {
			cells = append(cells, brPicCells(a.ls.snap, it)...)
			continue
		}
		fgc, bold := brItemStyle(it.kind, it.bold)
		for _, ch := range it.text {
			w := lipgloss.Width(string(ch))
			if w == 0 {
				continue
			}
			c := brCell{s: string(ch), w: w, fg: fgc, bold: bold, elem: it.elem}
			if it.elem >= 0 && it.elem == hover {
				c.fg, c.bg, c.bold = colorOnState, colorAccent, true
			}
			cells = append(cells, c)
			if w == 2 {
				cells = append(cells, brCell{s: "", w: 0, elem: it.elem})
			}
		}
	}
	for len(cells) <= a.ls.pcol {
		cells = append(cells, brCell{s: " ", w: 1, fg: colorText, elem: -1})
	}
	if r == a.ls.prow {
		i := a.ls.pcol
		if cells[i].w == 0 && i > 0 {
			i--
		}
		cells[i].fg, cells[i].bg = colorOnState, colorText
		if cells[i].s == "" {
			cells[i].s = " "
		}
	}
	var segs []seg
	used := 0
	for _, c := range cells {
		if c.w == 0 {
			continue
		}
		if used+c.w > tw {
			break
		}
		used += c.w
		if n := len(segs); n > 0 && segs[n-1].fg == c.fg && segs[n-1].bg == c.bg && segs[n-1].bold == c.bold {
			segs[n-1].t += c.s
			continue
		}
		segs = append(segs, seg{t: c.s, fg: c.fg, bg: c.bg, bold: c.bold})
	}
	return segs
}

// hoverElem 은 커서 아래에 있는 요소 번호 (없으면 -1)
func (a *browserApp) hoverElem() int {
	if a.ls.scr == nil || a.ls.snap == nil {
		return -1
	}
	if it, ok := a.ls.scr.itemAt(a.ls.prow, a.ls.pcol); ok && it.elem >= 0 && it.elem < len(a.ls.snap.Elems) {
		return it.elem
	}
	return -1
}

func (a *browserApp) livePos() string {
	s := a.ls.snap
	if s == nil || s.DH <= s.VH+1 {
		return "100%"
	}
	return fmt.Sprintf("%d%%", int(max(0, min(100, s.SY*100/(s.DH-s.VH)))))
}

// liveStatus 는 라이브 모드의 메시지 줄: 입력 중, 처리 중, 커서 아래 요소 안내 순
func (a *browserApp) liveStatus() (string, Tone, bool) {
	switch {
	case a.ls.edit:
		return "입력 중: 글자를 치고 Enter 로 제출, Esc 로 입력 끝", ToneAccent, true
	case a.ls.busy:
		return "처리 중 " + string("|/-\\"[a.frame%4]), ToneAccent, true
	}
	if a.msg != "" {
		return a.msg, a.msgTone, true
	}
	if i := a.hoverElem(); i >= 0 {
		e := a.ls.snap.Elems[i]
		switch {
		case e.Href != "":
			tone := ToneInfo
			if !brSameSite(a.cur.URL, e.Href) {
				tone = ToneWarn
			}
			return "→ " + e.Href, tone, true
		case e.Kind == "input":
			return "입력창: Enter 로 입력 시작", ToneInfo, true
		case e.Kind == "select":
			return "선택 상자: Enter 로 다음 항목", ToneInfo, true
		case e.Kind == "check":
			return "체크: Enter 로 켜고 끔", ToneInfo, true
		case e.Label != "":
			return strings.TrimSpace("누르기: " + e.Label), ToneInfo, true
		}
	}
	return "", ToneInfo, false
}

func (a *browserApp) onAppTick(m appTickMsg) tea.Cmd {
	if m.Gen != a.gen {
		return nil
	}
	switch m.Tag {
	case "load":
		if !a.loading {
			return nil
		}
		a.frame++
		return tickCmd(150*time.Millisecond, a.gen, "load")
	case "busy":
		if !a.ls.busy {
			return nil
		}
		a.frame++
		return tickCmd(150*time.Millisecond, a.gen, "busy")
	case "lrefresh":
		a.ls.refreshing = false
		if a.ls.on && !a.ls.busy && !a.loading {
			return a.act(brAct{kind: "snap"})
		}
	}
	return nil
}

// showRow 는 r 번째 줄이 화면에 보이도록 줄 창을 옮김
func (a *browserApp) showRow(r int) {
	l := &a.ls
	switch {
	case r < l.roff:
		l.roff = r
	case r >= l.roff+a.vh:
		l.roff = r - a.vh + 1
	}
	l.roff = max(0, min(l.roff, max(0, len(l.scr.rows)-a.vh)))
}

// pageMove 는 한 화면씩 넘김: 아직 안 보인 줄이 있으면 줄 창을 먼저 옮기고, 끝에 닿으면 페이지를 스크롤함
func (a *browserApp) pageMove(dir int) tea.Cmd {
	l := &a.ls
	step := max(1, a.vh-1)
	switch {
	case dir > 0 && l.roff+a.vh < len(l.scr.rows):
		l.roff = min(l.roff+step, len(l.scr.rows)-a.vh)
	case dir < 0 && l.roff > 0:
		l.roff = max(0, l.roff-step)
	default:
		return a.act(a.scrollAct(float64(dir) * a.pageDy()))
	}
	l.prow = max(l.roff, min(l.prow, l.roff+a.vh-1))
	return nil
}

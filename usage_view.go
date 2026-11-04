package main

import (
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Claude / Codex 사용량 앱이 함께 쓰는 부분 (F-08, F-09).
// 기록 읽기(캐시, 중복 제거, 집계)와 화면(탭, 표, 막대, 로딩, 빈 상태)을 여기서 맡고,
// 기록 형식별 파서는 usage_claude.go / usage_codex.go 에 있음. 기록 파일은 읽기만 하고 절대 수정하지 않음.

// ---- 기록 한 건 ----

// usageRec 은 응답 한 건의 토큰 사용량. 입력은 캐시 읽기를 뺀 값이고, 네 값은 서로 겹치지 않음.
type usageRec struct {
	Key            string // 같은 응답을 가리키는 키. 비어 있으면 중복 제거 대상이 아님
	TS             time.Time
	Model          string
	In, Out        int64
	CacheW, CacheR int64
}

// usageSource 는 기록 종류 하나(Claude Code, Codex)에 대한 설명
type usageSource struct {
	Name          string                              // 빈 상태 제목에 쓰는 이름
	EmptyHint     string                              // 빈 상태의 보조 안내
	Dir           func() string                       // 기록 폴더
	Parse         func(io.Reader) ([]usageRec, error) // 파일 하나를 읽어 기록 목록을 돌려줌 (읽을 수 없는 줄은 건너뜀)
	HasCacheWrite bool                                // 캐시 생성 토큰을 기록하는 형식인지 (아니면 표에 - 로 표시)
}

// ---- 파일 읽기와 캐시 ----

type usageFile struct {
	path string
	size int64
	mod  time.Time
}

type usageEntry struct {
	size int64
	mod  time.Time
	recs []usageRec
}

// usageCache 는 파일별 파싱 결과. 크기와 수정 시각이 같으면 다시 읽지 않음.
type usageCache struct {
	mu sync.Mutex
	m  map[string]usageEntry
}

func newUsageCache() *usageCache { return &usageCache{m: map[string]usageEntry{}} }

func (c *usageCache) get(f usageFile) ([]usageRec, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[f.path]
	if ok && e.size == f.size && e.mod.Equal(f.mod) {
		return e.recs, true
	}
	return nil, false
}

func (c *usageCache) put(f usageFile, recs []usageRec) {
	c.mu.Lock()
	c.m[f.path] = usageEntry{size: f.size, mod: f.mod, recs: recs}
	c.mu.Unlock()
}

func (c *usageCache) keep(paths map[string]bool) {
	c.mu.Lock()
	for p := range c.m {
		if !paths[p] {
			delete(c.m, p)
		}
	}
	c.mu.Unlock()
}

// usageData 는 한 번 읽은 결과 (화면은 이것만 봄)
type usageData struct {
	recs   []usageRec
	files  int // 찾은 기록 파일 수
	failed int // 열지 못한 파일 수
	reread int // 이번에 실제로 다시 읽은 파일 수
	at     time.Time
	dir    string
	err    error // 폴더를 읽을 수 없을 때
}

// usageClock 은 "마지막 갱신" 시각 (테스트에서 바꿀 수 있음)
var usageClock = time.Now

// usageWorkers 는 동시에 읽는 파일 수
var usageWorkers = 4

// usageList 는 폴더 아래 *.jsonl 을 모두 찾음. 폴더가 없으면 빈 목록, 그 밖의 오류는 err.
func usageList(dir string) ([]usageFile, error) {
	if dir == "" {
		return nil, nil
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []usageFile
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".jsonl") {
			return nil
		}
		if info, e := d.Info(); e == nil {
			out = append(out, usageFile{path: p, size: info.Size(), mod: info.ModTime()})
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, nil
}

// usageParseFile 은 파일을 읽기 전용으로 열어 파싱함
func usageParseFile(src *usageSource, path string) ([]usageRec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return src.Parse(f)
}

// usageLoad 는 진행 상태 (읽는 고루틴이 쓰고 화면이 읽음)
type usageLoad struct{ done, total atomic.Int64 }

// usageRun 은 기록을 모두 읽어 합친 결과를 돌려줌. 캐시에 있는 파일은 건너뜀.
func usageRun(src *usageSource, cache *usageCache, ld *usageLoad) *usageData {
	dir := src.Dir()
	d := &usageData{dir: dir, at: usageClock()}
	files, err := usageList(dir)
	if err != nil {
		d.err = err
		ld.total.Store(0)
		return d
	}
	d.files = len(files)
	ld.total.Store(int64(len(files)))
	paths := make(map[string]bool, len(files))
	for _, f := range files {
		paths[f.path] = true
	}
	cache.keep(paths)

	var reread, failed atomic.Int64
	jobs := make(chan usageFile)
	var wg sync.WaitGroup
	for i := 0; i < max(1, usageWorkers); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				if _, ok := cache.get(f); !ok {
					recs, err := usageParseFile(src, f.path)
					if err != nil {
						failed.Add(1)
						cache.mu.Lock()
						delete(cache.m, f.path)
						cache.mu.Unlock()
					} else {
						cache.put(f, recs)
						reread.Add(1)
					}
				}
				ld.done.Add(1)
			}
		}()
	}
	for _, f := range files {
		jobs <- f
	}
	close(jobs)
	wg.Wait()
	d.failed, d.reread = int(failed.Load()), int(reread.Load())

	// 파일 순서대로 합치면서 같은 응답(Key)은 한 건으로 만듦: 출력 토큰은 큰 쪽, 시각은 빠른 쪽
	idx := map[string]int{}
	for _, f := range files {
		recs, ok := cache.get(f)
		if !ok {
			continue
		}
		for _, r := range recs {
			if r.Key == "" {
				d.recs = append(d.recs, r)
				continue
			}
			if i, dup := idx[r.Key]; dup {
				p := &d.recs[i]
				p.Out, p.In = max(p.Out, r.Out), max(p.In, r.In)
				p.CacheW, p.CacheR = max(p.CacheW, r.CacheW), max(p.CacheR, r.CacheR)
				if r.TS.Before(p.TS) {
					p.TS = r.TS
				}
				continue
			}
			idx[r.Key] = len(d.recs)
			d.recs = append(d.recs, r)
		}
	}
	return d
}

// ---- 집계 ----

type usageSum struct{ In, Out, CacheW, CacheR int64 }

func (s *usageSum) add(r usageRec) {
	s.In += r.In
	s.Out += r.Out
	s.CacheW += r.CacheW
	s.CacheR += r.CacheR
}
func (s usageSum) all() int64 { return s.In + s.Out + s.CacheW + s.CacheR }

type usageRow struct {
	Model string
	usageSum
}

type usageDay struct {
	Label string
	V     int64 // 입력 + 출력
	Today bool
}

const usagePeriods = 3

var usagePeriodNames = []string{"최근 5시간", "오늘", "이번 주"}

type usageReport struct {
	rows   [usagePeriods][]usageRow
	totals [usagePeriods]usageSum
	days   [7]usageDay
}

// usageStarts 는 기간별 시작 시각: 최근 5시간 / 오늘(로컬 자정) / 이번 주(월요일 자정)
func usageStarts(now time.Time) (starts [usagePeriods]time.Time, today0 time.Time) {
	today0 = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	wd := (int(today0.Weekday()) + 6) % 7
	return [usagePeriods]time.Time{now.Add(-5 * time.Hour), today0, today0.AddDate(0, 0, -wd)}, today0
}

func buildUsageReport(recs []usageRec, now time.Time) usageReport {
	var rep usageReport
	starts, today0 := usageStarts(now)
	byModel := [usagePeriods]map[string]*usageSum{}
	for i := range byModel {
		byModel[i] = map[string]*usageSum{}
	}
	var dayV [7]int64
	for _, r := range recs {
		ts := r.TS.In(now.Location())
		for p := 0; p < usagePeriods; p++ {
			if ts.Before(starts[p]) {
				continue
			}
			s := byModel[p][r.Model]
			if s == nil {
				s = &usageSum{}
				byModel[p][r.Model] = s
			}
			s.add(r)
			rep.totals[p].add(r)
		}
		d0 := time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, now.Location())
		if back := int(math.Round(today0.Sub(d0).Hours() / 24)); back >= 0 && back < 7 {
			dayV[back] += r.In + r.Out
		}
	}
	for p := range byModel {
		for m, s := range byModel[p] {
			rep.rows[p] = append(rep.rows[p], usageRow{Model: m, usageSum: *s})
		}
		rows := rep.rows[p]
		sort.Slice(rows, func(i, j int) bool {
			if a, b := rows[i].all(), rows[j].all(); a != b {
				return a > b
			}
			return rows[i].Model < rows[j].Model
		})
	}
	for back := 0; back < 7; back++ {
		d := today0.AddDate(0, 0, -back)
		lbl := d.Format("01-02")
		if back == 0 {
			lbl += " 오늘"
		}
		rep.days[back] = usageDay{Label: lbl, V: dayV[back], Today: back == 0}
	}
	return rep
}

// ---- 숫자 모양 ----

func usageCommas(v int64) string {
	s := fmt.Sprint(v)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// usageCompact 는 좁은 표에 쓰는 줄임 표기 (1.2K, 3.4M, 5.6B)
func usageCompact(v int64) string {
	f := float64(v)
	switch {
	case v >= 1e9:
		return fmt.Sprintf("%.1fB", f/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.1fM", f/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.1fK", f/1e3)
	}
	return fmt.Sprint(v)
}

// usageK 는 일별 막대 값 (K 단위). 0 이면 -
func usageK(v int64) string {
	switch {
	case v <= 0:
		return "-"
	case v < 500:
		return "<1K"
	}
	return usageCommas((v+500)/1000) + "K"
}

// ---- 표 배치 ----

type usageTable struct {
	widths []int
	head   []string
}

// usageCells 는 합계 하나를 표 네 칸의 글자로 바꿈. 캐시 생성을 기록하지 않는 형식이면 - 로 표시함
func usageCells(s usageSum, hasCW, compact bool) []string {
	fm := usageCommas
	if compact {
		fm = usageCompact
	}
	cw := "-"
	if hasCW {
		cw = fm(s.CacheW)
	}
	return []string{fm(s.In), fm(s.Out), cw, fm(s.CacheR)}
}

// usageLayout 은 안쪽 폭(avail)에 맞는 열 폭을 고름. 숫자가 넓으면 줄임 표기와 짧은 머리글로 단계를 낮춰서
// 모델 이름 칸을 최대한 확보함.
func usageLayout(avail int, models []string, sums []usageSum, hasCW bool) (usageTable, bool) {
	longest := lipgloss.Width("합계")
	for _, m := range models {
		longest = max(longest, lipgloss.Width(m))
	}
	want := min(longest, 18)
	headFull := []string{"입력", "출력", "캐시 생성", "캐시 읽기"}
	headShort := []string{"입력", "출력", "생성", "읽기"}
	var t usageTable
	for level := 0; level < 3; level++ {
		head := headFull
		if level == 2 {
			head = headShort
		}
		num := make([]int, 4)
		for j := range num {
			num[j] = lipgloss.Width(head[j])
		}
		for _, s := range sums {
			for j, c := range usageCells(s, hasCW, level > 0) {
				num[j] = max(num[j], lipgloss.Width(c))
			}
		}
		total := 0
		for j := range num {
			num[j]++ // 열 사이 간격 1칸
			total += num[j]
		}
		modelW := avail - total
		if modelW >= want || level == 2 {
			modelW = max(6, min(modelW, longest+4))
			t = usageTable{widths: append([]int{modelW}, num...), head: append([]string{"모델"}, head...)}
			return t, level > 0
		}
	}
	return t, true
}

// ---- 앱 ----

type usageLoadedMsg struct {
	gen  int
	data *usageData
}

type usageApp struct {
	src    *usageSource
	gen    int
	period int
	cache  *usageCache
	ld     *usageLoad
	busy   bool
	data   *usageData // 마지막으로 읽은 결과 (처음 읽는 중이면 nil)

	repData *usageData
	repMin  int64
	rep     usageReport
}

func newUsageApp(src *usageSource) *usageApp {
	return &usageApp{src: src, gen: newGen(), period: 1, cache: newUsageCache(), ld: &usageLoad{}, busy: true}
}

// start 는 기록 읽기를 고루틴에서 시작함. 끝나면 usageLoadedMsg 가 옴.
func (a *usageApp) start() tea.Cmd {
	a.busy = true
	ld := &usageLoad{}
	a.ld = ld
	gen, src, cache := a.gen, a.src, a.cache
	ch := make(chan *usageData, 1)
	go func() { ch <- usageRun(src, cache, ld) }()
	return func() tea.Msg { return usageLoadedMsg{gen: gen, data: <-ch} }
}

func (a *usageApp) Init() tea.Cmd { return a.start() }

func (a *usageApp) Update(msg tea.Msg) (App, tea.Cmd) {
	switch msg := msg.(type) {
	case usageLoadedMsg:
		if msg.gen != a.gen {
			return a, nil
		}
		a.busy = false
		a.data = msg.data
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return a, goHome()
		case "left":
			a.period = max(0, a.period-1)
		case "right":
			a.period = min(usagePeriods-1, a.period+1)
		case "r":
			if !a.busy {
				return a, a.start()
			}
		}
	}
	return a, nil
}

// report 는 기간별 집계. 읽은 결과나 분이 바뀔 때만 다시 계산함 (화면은 250ms 마다 그려짐)
func (a *usageApp) report(now time.Time) *usageReport {
	if mn := now.Unix() / 60; a.repData != a.data || a.repMin != mn {
		a.repData, a.repMin = a.data, mn
		a.rep = buildUsageReport(a.data.recs, now)
	}
	return &a.rep
}

func usageDisplayPath(p string) string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		if rel, err := filepath.Rel(h, p); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
			p = filepath.Join("~", rel)
		}
	}
	return filepath.ToSlash(p)
}

// usageHeights 는 창 높이(h)에 맞춘 본문 배치: 모델 행 줄 수(models)와 일별 막대 일수(days).
// 늘 있는 줄은 상단 3 + 탭 1 + 머리글 1 + 합계 1 + 하단 5 = 11줄이고 (높이 24 이상이면 빈 줄 3개가 더 붙음),
// 나머지 자리에 막대 7일 + 모델 행을 넣음. 모자라면 먼저 모델 행을 3줄까지 두고 막대 일수를 3일까지 줄이고,
// 그래도 모자라면 모델 행을 줄이다가(0이면 합계만) 막대 일수를 더 줄임. 높이 14 이상이면 항상 창 안에 들어옴.
func usageHeights(h int) (models, days int) {
	free := h - 11
	if h >= 24 {
		free -= 3 // 빈 줄
	}
	models, days = min(8, free-7), 7
	if models < 3 {
		days = max(3, min(7, free-3))
		models = free - days
	}
	models = max(0, models)
	days = max(0, min(days, free-models))
	return models, days
}

func (a *usageApp) View(c Ctx) string {
	f := NewFrame(c)
	compact := c.H < 24 // 낮은 창에서는 빈 줄을 줄임
	models, days := usageHeights(c.H)
	gap := func() {
		if !compact {
			f.Blank()
		}
	}
	if a.data != nil && a.data.files == 0 {
		return a.viewEmpty(f, gap)
	}
	gap()
	f.Tabs("기간", usagePeriodNames, a.period)
	gap()

	var rows []usageRow
	var tot usageSum
	var rep *usageReport
	if a.data != nil {
		rep = a.report(c.Now)
		rows, tot = rep.rows[a.period], rep.totals[a.period]
	}
	a.tableRows(f, models, rows, tot, a.data != nil)
	gap()

	if rep != nil {
		a.barRows(f, rep, days)
	} else if days > 0 {
		a.loadRow(f)
		for i := 1; i < days; i++ {
			f.Blank()
		}
	}
	f.Rule()
	f.MenuInline([2]string{"←→", "기간"}, [2]string{"r", "새로고침"}, [2]string{"Esc", "홈으로"})
	f.Rule()
	text, tone := a.status(c)
	f.Message(truncW(text, f.In()-4), tone)
	return f.String()
}

// tableRows 는 머리글, 모델별 행, 합계 행. 기록을 아직 못 읽었으면 합계만 - 로 채움.
// budget 은 모델 행에 쓸 수 있는 줄 수 (넘치면 마지막 줄에 "그 밖 N개", 2줄 미만이면 모델 행을 모두 뺌).
func (a *usageApp) tableRows(f *Frame, budget int, rows []usageRow, tot usageSum, loaded bool) {
	avail := f.In() - 4
	shown := rows
	var rest usageSum
	if len(rows) > budget {
		shown = rows[:max(0, budget-1)]
		for _, r := range rows[len(shown):] {
			rest.In += r.In
			rest.Out += r.Out
			rest.CacheW += r.CacheW
			rest.CacheR += r.CacheR
		}
	}
	var names []string
	sums := []usageSum{tot}
	for _, r := range shown {
		names = append(names, r.Model)
		sums = append(sums, r.usageSum)
	}
	otherName := ""
	if len(rows) > len(shown) && budget >= 2 {
		otherName = fmt.Sprintf("그 밖 %d개", len(rows)-len(shown))
		names = append(names, otherName)
		sums = append(sums, rest)
	}
	t, compactNum := usageLayout(avail, names, sums, a.src.HasCacheWrite)
	f.Cells(t.widths, t.head, colorTextMuted, false)
	cells := func(s usageSum) []string { return usageCells(s, a.src.HasCacheWrite, compactNum) }
	for _, r := range shown {
		f.Cells(t.widths, append([]string{r.Model}, cells(r.usageSum)...), colorText, false)
	}
	if otherName != "" {
		f.Cells(t.widths, append([]string{otherName}, cells(rest)...), colorTextMuted, false)
	}
	if loaded && len(rows) == 0 && budget >= 1 {
		f.Row(sg("이 기간의 기록 없음", colorTextMuted))
	}
	totCells := []string{"-", "-", "-", "-"}
	if loaded {
		totCells = cells(tot)
	}
	f.Cells(t.widths, append([]string{"합계"}, totCells...), colorText, true)
}

// loadRow 는 처음 읽는 중의 진행 막대: `기록 읽는 중…  ━━━━━━━━━━ 312/475`
func (a *usageApp) loadRow(f *Frame) {
	done, total := a.ld.done.Load(), a.ld.total.Load()
	label := "기록 읽는 중…  "
	if total == 0 {
		f.Row(sg("기록 찾는 중…", colorTextMuted))
		return
	}
	cnt := fmt.Sprintf(" %d/%d", done, total)
	bw := max(8, min(40, f.In()-4-lipgloss.Width(label)-lipgloss.Width(cnt)))
	n := int(math.Floor(clamp01(float64(done)/float64(total)) * float64(bw)))
	f.Row(sg(label, colorTextMuted), sg(strings.Repeat("━", n), colorAccent), sg(strings.Repeat("━", bw-n), colorTextFaint), sg(cnt, colorText))
}

// barRows 는 최근 일별 막대 (입력 + 출력, K 단위). days 일만큼 오늘부터 거슬러 올라가며 그림 (최대 7일).
// 막대 길이는 안쪽 폭에서 남는 만큼으로 줄임.
func (a *usageApp) barRows(f *Frame, rep *usageReport, days int) {
	const labelW = 12
	var maxV int64
	textW := 1
	shown := rep.days[:min(days, len(rep.days))]
	for _, d := range shown {
		maxV = max(maxV, d.V)
		textW = max(textW, lipgloss.Width(usageK(d.V)))
	}
	barMax := max(4, min(28, f.In()-4-labelW-1-textW))
	for _, d := range shown {
		n := 0
		if d.V > 0 && maxV > 0 {
			n = max(1, int(math.Round(float64(d.V)/float64(maxV)*float64(barMax))))
		}
		col := colorTextMuted
		if d.Today {
			col = colorAccent
		}
		segs := []seg{sg(padR(d.Label, labelW), colorTextMuted)}
		if n > 0 {
			segs = append(segs, sg(strings.Repeat("█", n), col), sg(" ", colorText))
		}
		f.Row(append(segs, sg(usageK(d.V), colorText))...)
	}
}

// status 는 하단 메시지 줄
func (a *usageApp) status(c Ctx) (string, Tone) {
	d := a.data
	if d == nil {
		return "읽는 동안에도 ←→ 와 Esc 는 쓸 수 있음", ToneInfo
	}
	if a.busy {
		return fmt.Sprintf("다시 읽는 중… %d/%d", a.ld.done.Load(), a.ld.total.Load()), ToneInfo
	}
	txt := fmt.Sprintf("기록 파일 %d개를 읽음", d.files)
	if d.reread < d.files-d.failed {
		txt = fmt.Sprintf("기록 파일 %d개 중 %d개를 다시 읽음", d.files, d.reread)
	}
	txt += " · 마지막 갱신 " + d.at.Format("15:04:05")
	if d.failed > 0 {
		return fmt.Sprintf("읽지 못한 파일 %d개 · %s", d.failed, txt), ToneWarn
	}
	return txt, ToneInfo
}

// viewEmpty 는 기록 파일이 하나도 없을 때의 안내 화면 (앱은 정상 동작함)
func (a *usageApp) viewEmpty(f *Frame, gap func()) string {
	d := a.data
	gap()
	f.Row(sgBold(a.src.Name+" 기록을 찾을 수 없음", colorText))
	gap()
	path := truncW(usageDisplayPath(d.dir), f.In()-4-lipgloss.Width("찾은 위치  ")-lipgloss.Width(" (기록 파일 0개)"))
	f.Row(sg("찾은 위치  ", colorTextMuted), sg(path, colorText), sg(" (기록 파일 0개)", colorTextMuted))
	f.Row(sg(a.src.EmptyHint, colorTextMuted))
	for i := 0; i < 6; i++ {
		gap()
	}
	f.Rule()
	f.MenuInline([2]string{"r", "다시 찾기"}, [2]string{"Esc", "홈으로"})
	f.Rule()
	switch {
	case d.err != nil:
		f.Message(truncW("기록 폴더를 읽을 수 없음: "+d.err.Error(), f.In()-4), ToneDanger)
	case a.busy:
		f.Message("다시 찾는 중…", ToneInfo)
	default:
		f.Message("", ToneInfo)
	}
	return f.String()
}

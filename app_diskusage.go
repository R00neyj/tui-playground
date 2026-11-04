package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 디스크 사용량 트리맵 (F-12). 읽기 전용: 디스크의 어떤 것도 지우거나 옮기거나 바꾸지 않음.
//
// 구성: ① 스캐너(고루틴, 취소 가능, 심볼릭 링크·정션 따라가지 않음) ② 스퀘어파이드 트리맵 배치
// ③ 화면(트리맵 / 목록). 스캐너와 디렉터리 읽기 함수는 패키지 변수라서 테스트가 바꿔 끼움.

var diskInfo = AppInfo{
	ID:   "diskusage",
	Name: "디스크 사용량",
	Desc: "폴더별 용량을 트리맵으로 보기",
	New:  newDiskApp,
}

// ---- 임계값 (화면 규칙) ----

const (
	duFixedRows   = 12 // 맵/목록을 뺀 고정 줄 수: 윗부분 3 + 경로 1 + 구분선 1 + 상세 2 + 구분선 1 + 메뉴 1 + 구분선 1 + 메시지 1 + 아랫 테두리 1
	duMinMapRows  = 8  // 맵 높이가 이보다 작으면(창 높이 < 20) 목록으로 자동 전환
	duMinCells    = 6  // 이보다 작은 칸 수가 될 항목은 "기타" 로 묶음
	duMaxRects    = 24 // 개별 사각형 최대 수
	duMinShown    = 6  // 모두 작아도 최소한 이만큼은 개별로 보임
	duKeepFiles   = 200
	duMaxDepth    = 256
	duProgressGap = 100 * time.Millisecond
)

// ---- 트리 ----

type dnode struct {
	name, path string
	size       int64 // 폴더면 하위 합계 (겉보기 크기)
	files      int64 // 하위 파일 수 (기타 묶음이면 묶인 파일 수)
	isDir      bool
	isOther    bool // 작은 파일을 스캔 단계에서 하나로 묶은 가상 항목
	kids       []*dnode
	parent     *dnode
}

// seal 은 자식의 부모 연결, 합계, 정렬(크기 내림차순, 같으면 이름순)을 마무리함
func (n *dnode) seal() {
	n.size, n.files = 0, 0
	for _, k := range n.kids {
		k.parent = n
		n.size += k.size
		n.files += k.files
	}
	sortNodes(n.kids)
}

func sortNodes(ks []*dnode) {
	sort.SliceStable(ks, func(i, j int) bool {
		if ks[i].size != ks[j].size {
			return ks[i].size > ks[j].size
		}
		return ks[i].name < ks[j].name
	})
}

func nodeLabel(n *dnode) string {
	switch {
	case n.isOther:
		return fmt.Sprintf("기타 (%d개)", n.files)
	case n.isDir:
		return n.name + "/"
	}
	return n.name
}

// ---- 스캐너 ----

// scanProgress 는 스캔 고루틴이 갱신하고 화면이 100ms 마다 읽어가는 진행 상황
type scanProgress struct {
	files, bytes, skipped, links atomic.Int64
	mu                           sync.Mutex
	cur                          string
}

type progSnap struct {
	files, bytes, skipped, links int64
	cur                          string
}

func (p *scanProgress) setCur(s string) { p.mu.Lock(); p.cur = s; p.mu.Unlock() }
func (p *scanProgress) snapshot() progSnap {
	p.mu.Lock()
	cur := p.cur
	p.mu.Unlock()
	return progSnap{p.files.Load(), p.bytes.Load(), p.skipped.Load(), p.links.Load(), cur}
}

// 테스트가 바꿔 끼우는 지점들: 진짜 드라이브를 건드리지 않게 함
var (
	scanTree  = walkScan
	duReadDir = os.ReadDir
	duStat    = os.Stat
	duInfo    = func(e fs.DirEntry) (fs.FileInfo, error) { return e.Info() }
)

// walkScan 은 root 아래를 재귀로 훑어 트리를 만듦. 심볼릭 링크·정션(재해석 지점)은 따라가지 않고,
// 읽기 오류(권한 등)는 건너뛰면서 센다. ctx 가 취소되면 가능한 빨리 멈추고 ctx.Err() 를 돌려줌.
func walkScan(ctx context.Context, root string, p *scanProgress) (*dnode, error) {
	fi, err := duStat(root)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, errors.New("폴더가 아님")
	}
	n := &dnode{name: root, path: root, isDir: true}
	scanDir(ctx, n, p, 0)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return n, nil
}

func isLinkLike(m fs.FileMode) bool { return m&(fs.ModeSymlink|fs.ModeIrregular) != 0 }

func scanDir(ctx context.Context, n *dnode, p *scanProgress, depth int) {
	entries, err := duReadDir(n.path)
	if err != nil {
		p.skipped.Add(1)
	}
	var files []*dnode
	for _, e := range entries {
		if ctx.Err() != nil {
			return
		}
		t := e.Type()
		if isLinkLike(t) {
			p.links.Add(1)
			continue
		}
		switch {
		case e.IsDir():
			if runtime.GOOS == "windows" { // 정션은 DirEntry 종류에 안 드러나는 경우를 대비해 한 번 더 확인
				if info, err := duInfo(e); err == nil && isLinkLike(info.Mode()) {
					p.links.Add(1)
					continue
				}
			}
			if depth >= duMaxDepth {
				p.skipped.Add(1)
				continue
			}
			if depth == 0 {
				p.setCur(e.Name())
			}
			c := &dnode{name: e.Name(), path: filepath.Join(n.path, e.Name()), isDir: true}
			scanDir(ctx, c, p, depth+1)
			n.kids = append(n.kids, c)
		case t.IsRegular():
			info, err := duInfo(e)
			if err != nil {
				p.skipped.Add(1)
				continue
			}
			sz := info.Size()
			p.files.Add(1)
			p.bytes.Add(sz)
			files = append(files, &dnode{name: e.Name(), path: filepath.Join(n.path, e.Name()), size: sz, files: 1})
		}
	}
	if len(files) > duKeepFiles { // 메모리를 아끼려고 작은 파일은 하나로 합쳐 둠
		sortNodes(files)
		rest := files[duKeepFiles:]
		other := &dnode{name: "기타", path: n.path, isOther: true}
		for _, f := range rest {
			other.size += f.size
			other.files++
		}
		files = append(files[:duKeepFiles:duKeepFiles], other)
	}
	n.kids = append(n.kids, files...)
	n.seal()
}

// ---- 크기 표기 ----

func humanSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	v := float64(n)
	for _, u := range []string{"KB", "MB", "GB", "TB"} {
		v /= 1024
		if v < 1024 || u == "TB" {
			return fmt.Sprintf("%.1f %s", v, u)
		}
	}
	return ""
}

func commaInt(n int64) string {
	s := fmt.Sprint(n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// truncLeft 는 폭이 w 를 넘으면 앞을 잘라내고 … 를 붙임 (경로는 끝부분이 중요함)
func truncLeft(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	if w < 1 {
		return ""
	}
	rs := []rune(s)
	n := 0
	i := len(rs)
	for i > 0 {
		rw := lipgloss.Width(string(rs[i-1]))
		if n+rw > w-1 {
			break
		}
		n += rw
		i--
	}
	return "…" + string(rs[i:])
}

// ---- 스퀘어파이드 트리맵 배치 ----

type frect struct{ x0, y0, x1, y1 float64 }

func (r frect) w() float64 { return r.x1 - r.x0 }
func (r frect) h() float64 { return r.y1 - r.y0 }

func worstRatio(row []float64, sum, short float64) float64 {
	if sum <= 0 || short <= 0 {
		return math.Inf(1)
	}
	s2, t2 := sum*sum, short*short
	w := 0.0
	for _, a := range row {
		if a <= 0 {
			continue
		}
		w = math.Max(w, math.Max(t2*a/s2, s2/(t2*a)))
	}
	return w
}

// squarify 는 넓이가 areas(내림차순, 모두 양수, 합 = w*h) 인 사각형들을 (x,y,w,h) 안에 채움 (Bruls 외, 2000).
// 모서리 좌표를 그대로 공유해서 틈이나 겹침 없이 영역 전체를 덮음.
func squarify(areas []float64, x, y, w, h float64) []frect {
	out := make([]frect, len(areas))
	ex, ey := x+w, y+h
	i := 0
	for i < len(areas) {
		if i == len(areas)-1 {
			out[i] = frect{x, y, ex, ey}
			break
		}
		short := math.Min(ex-x, ey-y)
		sum := areas[i]
		cur := worstRatio(areas[i:i+1], sum, short)
		j := i + 1
		for j < len(areas) {
			ns := sum + areas[j]
			nw := worstRatio(areas[i:j+1], ns, short)
			if nw > cur {
				break
			}
			sum, cur = ns, nw
			j++
		}
		last := j == len(areas)
		if ex-x >= ey-y { // 가로가 길면 왼쪽에 세로 띠
			sw := sum / (ey - y)
			x1 := x + sw
			if last {
				x1 = ex
			}
			cy := y
			for k := i; k < j; k++ {
				y1 := cy + areas[k]/sw
				if k == j-1 {
					y1 = ey
				}
				out[k] = frect{x, cy, x1, y1}
				cy = y1
			}
			x = x1
		} else { // 세로가 길면 위쪽에 가로 띠
			sh := sum / (ex - x)
			y1 := y + sh
			if last {
				y1 = ey
			}
			cx := x
			for k := i; k < j; k++ {
				x1 := cx + areas[k]/sh
				if k == j-1 {
					x1 = ex
				}
				out[k] = frect{cx, y, x1, y1}
				cx = x1
			}
			y = y1
		}
		i = j
	}
	return out
}

// cellRect 는 칸 단위 사각형 [x0,x1) x [y0,y1)
type cellRect struct{ x0, y0, x1, y1 int }

func (r cellRect) w() int      { return r.x1 - r.x0 }
func (r cellRect) h() int      { return r.y1 - r.y0 }
func (r cellRect) empty() bool { return r.w() <= 0 || r.h() <= 0 }
func (r cellRect) area() int   { return max(0, r.w()) * max(0, r.h()) }

// layoutCells 는 sizes(크기 내림차순을 권장) 를 W x H 칸에 배치함. 칸은 세로가 가로의 약 2배라서
// 가로 1칸 = 1, 세로 1줄 = 2 단위인 좌표계로 계산한 뒤 모서리를 반올림해 칸에 맞춤.
// 이웃한 사각형이 같은 모서리 값을 반올림하므로 틈이나 겹침이 없음. 크기가 0 이하인 항목은 빈 사각형.
func layoutCells(sizes []int64, W, H int) []cellRect {
	out := make([]cellRect, len(sizes))
	if W <= 0 || H <= 0 {
		return out
	}
	var total float64
	var idx []int
	for i, s := range sizes {
		if s > 0 {
			total += float64(s)
			idx = append(idx, i)
		}
	}
	if total == 0 {
		return out
	}
	hu := float64(H) * 2
	areas := make([]float64, len(idx))
	for k, i := range idx {
		areas[k] = float64(sizes[i]) / total * float64(W) * hu
	}
	for k, fr := range squarify(areas, 0, 0, float64(W), hu) {
		out[idx[k]] = cellRect{
			int(math.Round(fr.x0)), int(math.Round(fr.y0 / 2)),
			int(math.Round(fr.x1)), int(math.Round(fr.y1 / 2)),
		}
	}
	return out
}

// ---- 한 폴더의 화면용 항목 ----

// tmEntry 는 트리맵의 사각형 하나. kid 가 nil 이면 "기타" 묶음(kids[lo:hi])
type tmEntry struct {
	lo, hi int
	kid    *dnode
	size   int64
	count  int64 // 묶음일 때 파일 수
	rect   cellRect
	col    lipgloss.Color
}

func (e tmEntry) label() string {
	if e.kid == nil {
		return fmt.Sprintf("기타 (%d개)", e.count)
	}
	return nodeLabel(e.kid)
}

// buildEntries 는 n 의 자식을 W x H 칸의 트리맵 항목으로 만듦.
// 칸 수가 duMinCells 보다 작아질 항목과 duMaxRects 를 넘는 항목은 "기타" 하나로 묶음.
func buildEntries(n *dnode, W, H int) []tmEntry {
	if n == nil || len(n.kids) == 0 || n.size <= 0 || W <= 0 || H <= 0 {
		return nil
	}
	kids := n.kids
	cells := float64(W * H)
	k := 0
	for k < len(kids) && k < duMaxRects && float64(kids[k].size)*cells/float64(n.size) >= duMinCells {
		k++
	}
	k = max(k, min(duMinShown, len(kids)))
	if len(kids)-k == 1 && k < duMaxRects+1 {
		k++ // 하나만 남으면 묶지 않고 그대로 보임
	}
	k = min(k, len(kids))
	var es []tmEntry
	for i := 0; i < k; i++ {
		es = append(es, tmEntry{lo: i, hi: i + 1, kid: kids[i], size: kids[i].size, count: kids[i].files})
	}
	if k < len(kids) {
		g := tmEntry{lo: k, hi: len(kids)}
		for _, c := range kids[k:] {
			g.size += c.size
			g.count += c.files
		}
		es = append(es, g)
	}
	// 배치는 크기 내림차순이 가장 좋은 모양이 나옴 (묶음이 개별 항목보다 클 수 있음)
	order := make([]int, len(es))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return es[order[a]].size > es[order[b]].size })
	// 아주 작은 항목도 최소 duMinCells 칸은 차지하게 배치 크기에 바닥값을 둠 (보이지 않으면 고를 수 없으므로)
	floor := int64(math.Ceil(float64(n.size) * duMinCells / cells))
	sizes := make([]int64, len(es))
	for pos, i := range order {
		sizes[pos] = es[i].size
		if sizes[pos] > 0 {
			sizes[pos] = max(sizes[pos], floor)
		}
	}
	for pos, r := range layoutCells(sizes, W, H) {
		es[order[pos]].rect = r
	}
	colorEntries(es)
	return es
}

var vizPalette = []lipgloss.Color{colorViz1, colorViz2, colorViz3, colorViz4, colorViz5, colorViz6}

func touching(a, b cellRect) bool {
	if a.empty() || b.empty() {
		return false
	}
	ox := min(a.x1, b.x1) - max(a.x0, b.x0)
	oy := min(a.y1, b.y1) - max(a.y0, b.y0)
	return (ox > 0 && (a.y1 == b.y0 || b.y1 == a.y0)) || (oy > 0 && (a.x1 == b.x0 || b.x1 == a.x0))
}

// colorEntries 는 맞닿은 사각형끼리 다른 색이 되도록 팔레트에서 하나씩 고름 (탐욕 채색)
func colorEntries(es []tmEntry) {
	for i := range es {
		used := map[lipgloss.Color]int{}
		for j := 0; j < i; j++ {
			if touching(es[i].rect, es[j].rect) {
				used[es[j].col]++
			}
		}
		best, bestN := 0, 1<<30
		for c := 0; c < len(vizPalette); c++ {
			p := (i + c) % len(vizPalette)
			if n := used[vizPalette[p]]; n < bestN {
				best, bestN = p, n
			}
		}
		es[i].col = vizPalette[best]
	}
}

// moveEntry 는 from 에서 dir(0 위, 1 아래, 2 왼쪽, 3 오른쪽) 방향으로 가장 가까운 사각형을 고름.
// 점수 = 방향 쪽 간격 + 직각 방향으로 겹치지 않는 간격 x2 + 중심의 직각 방향 차이 x0.25 (세로는 1줄 = 2 단위)
func moveEntry(es []tmEntry, from, dir int) int {
	vis := func(i int) bool { return !es[i].rect.empty() }
	if from < 0 || from >= len(es) || !vis(from) {
		for i := range es {
			if vis(i) {
				return i
			}
		}
		return from
	}
	type box struct{ x0, x1, y0, y1 float64 }
	bx := func(r cellRect) box { return box{float64(r.x0), float64(r.x1), float64(r.y0) * 2, float64(r.y1) * 2} }
	c := bx(es[from].rect)
	cx, cy := (c.x0+c.x1)/2, (c.y0+c.y1)/2
	gap := func(a0, a1, b0, b1 float64) float64 { return math.Max(0, math.Max(b0-a1, a0-b1)) }
	best, bestS := from, math.Inf(1)
	for i := range es {
		if i == from || !vis(i) {
			continue
		}
		o := bx(es[i].rect)
		ox, oy := (o.x0+o.x1)/2, (o.y0+o.y1)/2
		var along, perp, off float64
		switch dir {
		case 0:
			if oy >= cy {
				continue
			}
			along, perp, off = math.Max(0, c.y0-o.y1), gap(c.x0, c.x1, o.x0, o.x1), math.Abs(ox-cx)
		case 1:
			if oy <= cy {
				continue
			}
			along, perp, off = math.Max(0, o.y0-c.y1), gap(c.x0, c.x1, o.x0, o.x1), math.Abs(ox-cx)
		case 2:
			if ox >= cx {
				continue
			}
			along, perp, off = math.Max(0, c.x0-o.x1), gap(c.y0, c.y1, o.y0, o.y1), math.Abs(oy-cy)
		default:
			if ox <= cx {
				continue
			}
			along, perp, off = math.Max(0, o.x0-c.x1), gap(c.y0, c.y1, o.y0, o.y1), math.Abs(oy-cy)
		}
		if s := along + 2*perp + 0.25*off; s < bestS {
			best, bestS = i, s
		}
	}
	return best
}

// ---- 앱 ----

type duState int

const (
	stIdle   duState = iota // 아직 스캔을 시작하지 않음
	stScan                  // 스캔 중
	stDone                  // 트리를 가지고 있음
	stCancel                // 취소됨 (트리 없음)
	stFail                  // 실패 (트리 없음)
)

type duPhase int

const (
	dpMain duPhase = iota
	dpInput
)

type scanResult struct {
	root *dnode
	err  error
}

type scanDoneMsg struct {
	gen int
	scanResult
}

type duApp struct {
	ph      duPhase
	st      duState
	home    string // 열 때 기본으로 훑는 경로
	top     *dnode // 스캔한 트리의 뿌리 (스캔에 성공하기 전까지는 nil)
	cur     *dnode // 지금 보는 폴더
	sel     int    // cur.kids 에서 선택한 위치
	list    bool   // 사용자가 목록 보기를 골랐는지
	touched bool   // Esc 가 아닌 키를 한 번이라도 눌렀는지
	c       Ctx    // 마지막으로 그린 창 크기 (호스트가 WindowSizeMsg 를 앱에 주지 않아서 View 에서 기억함)
	msg     string
	msgTone Tone
	input   textinput.Model
	// 스캔 상태
	gen      int
	cancel   context.CancelFunc
	done     chan struct{} // 스캔 고루틴이 끝나면 닫힘
	prog     *scanProgress
	snap     progSnap
	spin     int
	scanPath string
	skipped  int64
	links    int64
}

func newDiskApp(env *Env) App {
	ti := textinput.New()
	ti.Prompt = "경로 > "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent)
	ti.CharLimit = 260
	ti.Width = 40
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return &duApp{home: home, input: ti, c: Ctx{W: 80, H: 24}}
}

func (a *duApp) Init() tea.Cmd { return a.startScan(a.home) }

func (a *duApp) say(s string, tone Tone) { a.msg, a.msgTone = s, tone }

// startScan 은 고루틴에서 path 를 훑기 시작함. 고루틴은 결과를 버퍼 1짜리 채널에 넣고 done 을 닫으며 끝남.
func (a *duApp) startScan(path string) tea.Cmd {
	a.stopScan()
	ctx, cancel := context.WithCancel(context.Background())
	a.gen = newGen()
	a.cancel = cancel
	a.prog = &scanProgress{}
	a.snap, a.spin = progSnap{}, 0
	a.scanPath = path
	a.st = stScan
	a.msg = ""
	gen, prog, fn := a.gen, a.prog, scanTree
	res := make(chan scanResult, 1)
	done := make(chan struct{})
	a.done = done
	go func() {
		defer close(done)
		defer cancel()
		n, err := fn(ctx, path, prog)
		res <- scanResult{n, err}
	}()
	wait := func() tea.Msg { return scanDoneMsg{gen: gen, scanResult: <-res} }
	return tea.Batch(wait, tickCmd(duProgressGap, gen, "scan"))
}

// stopScan 은 실행 중인 스캔을 취소하고, 이후에 도착하는 틱·결과 메시지를 무시하게 만듦
func (a *duApp) stopScan() {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.gen = 0
	if a.st == stScan {
		if a.top != nil {
			a.st = stDone
		} else {
			a.st = stCancel
		}
	}
}

func (a *duApp) Update(msg tea.Msg) (App, tea.Cmd) {
	switch m := msg.(type) {
	case appTickMsg:
		if m.Tag != "scan" || m.Gen != a.gen || a.st != stScan {
			return a, nil
		}
		a.snap = a.prog.snapshot()
		a.spin++
		return a, tickCmd(duProgressGap, a.gen, "scan")

	case scanDoneMsg:
		if m.gen != a.gen || a.st != stScan {
			return a, nil
		}
		a.cancel, a.gen = nil, 0
		s := a.prog.snapshot()
		if m.err != nil {
			if a.top != nil {
				a.st = stDone
			} else {
				a.st = stFail
			}
			a.say("스캔 실패: "+m.err.Error(), ToneDanger)
			return a, nil
		}
		a.top, a.cur, a.sel = m.root, m.root, 0
		a.skipped, a.links = s.skipped, s.links
		a.st = stDone
		a.say("", ToneInfo)
		return a, nil

	case tea.KeyMsg:
		return a.key(m)
	}
	if a.ph == dpInput { // 커서 깜박임 등 입력창 내부 메시지
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		return a, cmd
	}
	return a, nil
}

func (a *duApp) key(m tea.KeyMsg) (App, tea.Cmd) {
	k := m.String()
	if a.ph == dpInput { // 입력 중에는 Esc/Enter 만 특별하고 나머지 키는 모두 입력창이 받음
		switch k {
		case "esc":
			a.ph = dpMain
			return a, nil
		case "enter":
			return a.submitPath()
		}
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(m)
		return a, cmd
	}
	a.msg = ""
	if k != "esc" && k != "backspace" {
		a.touched = true
	}
	switch k {
	case "esc":
		switch a.st {
		case stScan:
			a.stopScan()
			if !a.touched { // 아무것도 안 눌렀으면 Esc 한 번으로 바로 홈
				return a, goHome()
			}
			a.say("스캔 취소됨", ToneWarn)
			return a, nil
		case stDone:
			if a.cur != nil && a.cur.parent != nil {
				a.ascend()
				return a, nil
			}
		}
		return a, goHome()
	case "backspace":
		if a.st == stDone && a.cur != nil && a.cur.parent != nil {
			a.ascend()
		}
		return a, nil
	case "q":
		a.stopScan()
		return a, goHome()
	case "c":
		if a.st == stScan {
			a.stopScan()
			a.say("스캔 취소됨", ToneWarn)
		}
		return a, nil
	case "p":
		if a.st == stScan {
			a.say("스캔 중에는 경로를 바꿀 수 없음 (c 로 취소)", ToneWarn)
			return a, nil
		}
		a.ph = dpInput
		a.input.SetValue("")
		return a, a.input.Focus()
	case "r":
		if a.st != stScan {
			p := a.scanPath
			if p == "" {
				p = a.home
			}
			return a, a.startScan(p)
		}
		return a, nil
	case "v":
		a.list = !a.list
		a.fixSel()
		return a, nil
	}
	if a.st != stDone || a.cur == nil {
		return a, nil
	}
	switch k {
	case "enter":
		a.descend()
	case "up", "k":
		a.move(0)
	case "down", "j":
		a.move(1)
	case "left", "h":
		a.move(2)
	case "right", "l":
		a.move(3)
	}
	return a, nil
}

func (a *duApp) submitPath() (App, tea.Cmd) {
	p := strings.Trim(strings.TrimSpace(a.input.Value()), `"`)
	a.ph = dpMain
	if p == "" {
		return a, nil
	}
	if len(p) == 2 && p[1] == ':' { // "C:" -> "C:\"
		p += `\`
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return a, a.startScan(filepath.Clean(p))
}

// ---- 이동 ----

func (a *duApp) mapSize(c Ctx) (w, h int, tree bool) {
	w, h = c.In()-4, c.H-duFixedRows
	return w, h, h >= duMinMapRows
}

func (a *duApp) useTree() bool {
	_, _, ok := a.mapSize(a.c)
	return ok && !a.list
}

func (a *duApp) entries() []tmEntry {
	w, h, _ := a.mapSize(a.c)
	return buildEntries(a.cur, w, h)
}

func entryOf(es []tmEntry, sel int) int {
	for i, e := range es {
		if sel >= e.lo && sel < e.hi {
			return i
		}
	}
	return -1
}

// fixSel 은 선택 위치를 범위 안으로 되돌림
func (a *duApp) fixSel() {
	if a.cur == nil || len(a.cur.kids) == 0 {
		a.sel = 0
		return
	}
	a.sel = max(0, min(a.sel, len(a.cur.kids)-1))
}

func (a *duApp) move(dir int) {
	if len(a.cur.kids) == 0 {
		return
	}
	if !a.useTree() {
		switch dir {
		case 0:
			a.sel = max(0, a.sel-1)
		case 1:
			a.sel = min(len(a.cur.kids)-1, a.sel+1)
		}
		return
	}
	es := a.entries()
	if len(es) == 0 {
		return
	}
	i := moveEntry(es, entryOf(es, a.sel), dir)
	if i >= 0 && i < len(es) {
		a.sel = es[i].lo
	}
}

func (a *duApp) descend() {
	if len(a.cur.kids) == 0 {
		return
	}
	var kid *dnode
	if a.useTree() {
		es := a.entries()
		i := entryOf(es, a.sel)
		if i < 0 || es[i].kid == nil {
			a.say("기타 묶음은 목록(v)에서 하나씩 열 수 있음", ToneInfo)
			return
		}
		kid = es[i].kid
	} else {
		kid = a.cur.kids[a.sel]
	}
	switch {
	case kid.isOther:
		a.say("작은 파일을 묶은 항목이라 열 수 없음", ToneInfo)
	case !kid.isDir:
		a.say("파일은 열 수 없음", ToneInfo)
	default:
		a.cur, a.sel = kid, 0
	}
}

func (a *duApp) ascend() {
	from := a.cur
	a.cur = from.parent
	a.sel = 0
	for i, k := range a.cur.kids {
		if k == from {
			a.sel = i
			break
		}
	}
}

// ---- 화면 ----

func (a *duApp) View(c Ctx) string {
	a.c = c
	if a.ph == dpInput {
		return a.viewInput(c)
	}
	if a.st == stDone && a.cur != nil {
		return a.viewTree(c)
	}
	return a.viewScan(c)
}

func (a *duApp) msgRow(f *Frame, def string) {
	text, tone := a.msg, a.msgTone
	if text == "" {
		text, tone = def, ToneInfo
	}
	f.Message(truncW(text, f.In()-4), tone)
}

const duNotice = "겉보기 크기 기준 (디스크 할당 크기 아님)"

func (a *duApp) viewScan(c Ctx) string {
	f := NewFrame(c).Blank()
	in := f.In() - 4
	switch a.st {
	case stScan:
		spin := string("|/-\\"[a.spin%4])
		s := a.snap
		f.Row(sg(spin+" ", colorAccent), sgBold("폴더 크기 계산 중", colorText))
		f.Row(sg(truncW(a.scanPath, in), colorTextMuted))
		f.Row(sg("파일  ", colorTextMuted), sg(commaInt(s.files)+"개   "+humanSize(s.bytes), colorText))
		cur := s.cur
		if cur == "" {
			cur = "-"
		}
		f.Row(sg("현재  ", colorTextMuted), sg(truncW(cur, in-6), colorText))
		skip := fmt.Sprintf("건너뜀 %d곳", s.skipped)
		f.Row(sg(skip, colorTextMuted))
	case stCancel:
		f.Row(sg("스캔이 취소됨", colorWarn))
		f.Row(sg("[r] 다시 스캔  [p] 다른 경로", colorTextMuted))
	case stFail:
		f.Row(sg("스캔하지 못함", colorDanger))
		f.Row(sg("[r] 다시 스캔  [p] 다른 경로", colorTextMuted))
	default:
		f.Row(sg("스캔 준비 중", colorTextMuted))
	}
	f.Blank().Rule()
	if a.st == stScan {
		f.MenuInline([2]string{"c / Esc", "스캔 취소"})
	} else {
		f.MenuInline([2]string{"r", "다시 스캔"}, [2]string{"p", "경로"}, [2]string{"Esc", "홈으로"})
	}
	f.Rule()
	a.msgRow(f, duNotice)
	return f.String()
}

func (a *duApp) viewInput(c Ctx) string {
	f := NewFrame(c)
	f.Row(sgBold("스캔할 경로", colorAccent)).Rule()
	f.Row(sg("현재  ", colorTextMuted), sg(truncW(a.scanPathOrHome(), f.In()-10), colorText))
	f.Blank()
	f.Row(sg("입력 예  ", colorTextMuted), sg(`C:\`, colorText), sg("  /  ", colorTextMuted), sg(`D:\Works`, colorText), sg("  /  ", colorTextMuted), sg("/home/me", colorText))
	f.Row(sg("Enter = 스캔 시작    Esc = 뒤로", colorTextMuted))
	f.Below("", a.input.View())
	return f.String()
}

func (a *duApp) scanPathOrHome() string {
	if a.scanPath != "" {
		return a.scanPath
	}
	return a.home
}

func (a *duApp) viewTree(c Ctx) string {
	f := NewFrame(c)
	in := f.In() - 4
	cur := a.cur
	right := humanSize(cur.size)
	if a.skipped > 0 {
		right += fmt.Sprintf("  건너뜀 %d곳", a.skipped)
	}
	f.RowLR([]seg{sg(truncLeft(cur.path, in-lipgloss.Width(right)-2), colorText)}, []seg{sg(right, colorTextMuted)})
	f.Rule()

	w, h, ok := a.mapSize(c)
	tree := ok && !a.list
	var es []tmEntry
	switch {
	case len(cur.kids) == 0 || cur.size == 0:
		f.Row(sg("비어 있음 (0 B)", colorTextMuted))
	case tree:
		es = buildEntries(cur, w, h)
		a.renderMap(f, es, w, h, entryOf(es, a.sel))
	default:
		a.renderList(f, c)
	}

	a.renderDetail(f, es, tree)
	f.Rule()
	f.MenuInline([2]string{"←↑↓→", "이동"}, [2]string{"Enter", "열기"}, [2]string{"v", "맵/목록"}, [2]string{"p", "경로"}, [2]string{"Esc", "뒤로"})
	f.Rule()
	def := duNotice
	if !ok && !a.list {
		def = "창이 작아 목록으로 표시함 / " + duNotice
	}
	a.msgRow(f, def)
	return f.String()
}

// renderMap 은 항목마다 배경색 칸으로 사각형을 그림. 선택한 사각형은 흰 배경 + `▸` + 굵은 글자
func (a *duApp) renderMap(f *Frame, es []tmEntry, W, H, sel int) {
	grid := make([][]int, H)
	for y := range grid {
		grid[y] = make([]int, W)
		for x := range grid[y] {
			grid[y][x] = -1
		}
	}
	for i, e := range es {
		r := e.rect
		for y := max(0, r.y0); y < min(H, r.y1); y++ {
			for x := max(0, r.x0); x < min(W, r.x1); x++ {
				grid[y][x] = i
			}
		}
	}
	for y := 0; y < H; y++ {
		var segs []seg
		for x := 0; x < W; {
			i := grid[y][x]
			x1 := x
			for x1 < W && grid[y][x1] == i {
				x1++
			}
			if i < 0 {
				segs = append(segs, sg(sp(x1-x), colorText))
			} else {
				segs = append(segs, cellSeg(es[i], i == sel, y-es[i].rect.y0, x1-x))
			}
			x = x1
		}
		f.Row(segs...)
	}
}

// cellSeg 는 사각형의 한 줄(폭 w) 을 그림. 0줄째 이름, 1줄째 크기, 2줄째 부모 대비 비율은 따로 안 넣음
func cellSeg(e tmEntry, selected bool, ry, w int) seg {
	mark := " "
	if selected && ry == 0 {
		mark = "▸"
	}
	text := ""
	switch ry {
	case 0:
		text = e.label()
	case 1:
		text = humanSize(e.size)
	}
	body := ""
	if text != "" && w-1 >= 3 {
		body = truncW(text, w-1)
	}
	s := seg{t: padR(mark+body, w), fg: colorOnState, bg: e.col}
	if selected {
		// 선택은 흰 배경 대신 색을 뒤집음: 검은 바탕에 그 사각형의 색 글자 (큰 흰 면이 눈부시지 않게)
		s.fg, s.bg, s.bold = e.col, colorOnState, true
	}
	return s
}

// renderList 는 크기순 막대 목록을 그림. 선택 항목이 보이도록 창을 옮김
func (a *duApp) renderList(f *Frame, c Ctx) {
	kids := a.cur.kids
	n := min(len(kids), max(1, min(30, c.H-duFixedRows)))
	start := max(0, min(a.sel-n/2, len(kids)-n))
	big := float64(kids[0].size)
	// BarRow 는 막대가 최대 28칸이라, 값 글자를 넣으면 좁은 창에서 넘침 -> 남는 폭에 맞춰 max 를 키움
	avail := float64(f.In() - 4 - 12 - 1 - 9)
	scaled := big * barRowMax / math.Max(1, math.Min(barRowMax, avail))
	for i := start; i < start+n; i++ {
		k := kids[i]
		mark := "  "
		if i == a.sel {
			mark = "▸ "
		}
		f.BarRow(truncW(mark+nodeLabel(k), 12), float64(k.size), scaled, humanSize(k.size), i == a.sel)
	}
}

// renderDetail 은 선택 항목의 이름·크기·부모 대비 비율 한 줄과 전체 경로 한 줄을 그림
func (a *duApp) renderDetail(f *Frame, es []tmEntry, tree bool) {
	in := f.In() - 4
	cur := a.cur
	if len(cur.kids) == 0 || cur.size == 0 {
		f.Row(sg("", colorText))
		f.Row(sg(truncLeft(cur.path, in), colorTextMuted))
		return
	}
	name, path, size := "", cur.path, int64(0)
	if i := entryOf(es, a.sel); tree && i >= 0 {
		e := es[i]
		name, size = e.label(), e.size
		if e.kid != nil {
			path = e.kid.path
		}
	} else if a.sel >= 0 && a.sel < len(cur.kids) {
		k := cur.kids[a.sel]
		name, size = nodeLabel(k), k.size
		if !k.isOther {
			path = k.path
		}
	}
	right := fmt.Sprintf("%s  부모의 %.1f%%", humanSize(size), float64(size)*100/float64(cur.size))
	lw := in - lipgloss.Width(right) - 2 - 2
	f.RowLR([]seg{sg("▸ ", colorAccent), sgBold(truncW(name, lw), colorText)}, []seg{sg(right, colorTextMuted)})
	f.Row(sg(truncLeft(path, in), colorTextMuted))
}

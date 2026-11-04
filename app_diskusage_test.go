package main

import (
	"context"
	"fmt"
	"io/fs"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---- 도우미 ----

// 테스트 트리 만들기 (디스크를 건드리지 않음)
func tFile(name string, size int64) *dnode { return &dnode{name: name, size: size, files: 1} }
func tDir(name string, kids ...*dnode) *dnode {
	n := &dnode{name: name, isDir: true, kids: kids}
	n.seal()
	return n
}
func setPaths(n *dnode, path string) {
	n.path = path
	for _, k := range n.kids {
		setPaths(k, filepath.Join(path, k.name))
	}
}

func newDU(t *testing.T) *duApp {
	t.Helper()
	return newDiskApp(&Env{}).(*duApp)
}

// ready 는 트리를 이미 읽은 상태의 앱을 만듦
func ready(t *testing.T, root *dnode, w, h int) *duApp {
	t.Helper()
	a := newDU(t)
	setPaths(root, "/root")
	a.top, a.cur, a.st = root, root, stDone
	a.c = testCtx(w, h)
	return a
}

// manyKids 는 긴 한글 이름·작은 파일이 섞인 40여 개 항목을 가진 폴더
func manyKids() *dnode {
	var ks []*dnode
	sz := int64(1 << 30)
	for i := 0; i < 24; i++ {
		name := fmt.Sprintf("아주긴한글폴더이름번호%02d_abcdefghijklmnopqrstuvwxyz", i)
		if i%2 == 0 {
			ks = append(ks, tDir(name, tFile("a.bin", sz/2), tFile("b.bin", sz/2)))
		} else {
			ks = append(ks, tFile(name+".dat", sz))
		}
		sz = sz * 85 / 100
	}
	for i := 0; i < 20; i++ {
		ks = append(ks, tFile(fmt.Sprintf("작은파일%d.txt", i), int64(100+i)))
	}
	return tDir("root", ks...)
}

func lineCount(v string) int { return strings.Count(v, "\n") + 1 }

// drive 는 명령을 실행하고 메시지를 앱에 넣는 이벤트 루프. 스캔이 끝날 때까지 돌림
func drive(t *testing.T, a App, cmd tea.Cmd) App {
	t.Helper()
	msgs := make(chan tea.Msg, 256)
	var run func(c tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		go func() {
			m := c()
			if b, ok := m.(tea.BatchMsg); ok {
				for _, s := range b {
					run(s)
				}
				return
			}
			msgs <- m
		}()
	}
	run(cmd)
	deadline := time.After(10 * time.Second)
	for {
		select {
		case m := <-msgs:
			var c tea.Cmd
			a, c = a.Update(m)
			run(c)
			if d := a.(*duApp); d.st != stScan {
				return a
			}
		case <-deadline:
			t.Fatal("스캔이 끝나지 않음")
		}
	}
}

func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("%s: 1초 안에 끝나지 않음 (고루틴 누수?)", what)
	}
}

// blockingScanner 는 ctx 가 취소될 때까지 기다리는 가짜 스캐너. 끝나면 exited 가 닫힘
func blockingScanner(t *testing.T) (started, exited chan struct{}) {
	t.Helper()
	started, exited = make(chan struct{}), make(chan struct{})
	old := scanTree
	scanTree = func(ctx context.Context, root string, p *scanProgress) (*dnode, error) {
		defer close(exited)
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	t.Cleanup(func() { scanTree = old })
	return
}

// ---- 배치 ----

func gridCover(t *testing.T, rs []cellRect, W, H int) {
	t.Helper()
	cnt := make([]int, W*H)
	for i, r := range rs {
		if r.x0 < 0 || r.y0 < 0 || r.x1 > W || r.y1 > H || r.x1 < r.x0 || r.y1 < r.y0 {
			t.Fatalf("사각형 %d 가 영역을 벗어남: %+v (%dx%d)", i, r, W, H)
		}
		for y := r.y0; y < r.y1; y++ {
			for x := r.x0; x < r.x1; x++ {
				cnt[y*W+x]++
			}
		}
	}
	for i, c := range cnt {
		if c != 1 {
			t.Fatalf("칸 (%d,%d) 가 %d번 덮임 (0=틈, 2 이상=겹침) rects=%+v", i%W, i/W, c, rs)
		}
	}
}

func TestSquarifyTilesExactly(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	dims := [][2]int{{46, 6}, {46, 10}, {68, 29}, {1, 1}, {3, 40}, {100, 3}, {20, 20}}
	for _, d := range dims {
		for trial := 0; trial < 60; trial++ {
			n := 1 + rng.Intn(25)
			sizes := make([]int64, n)
			for i := range sizes {
				sizes[i] = int64(1 + rng.Intn(1000)*rng.Intn(1000))
			}
			sortInt64Desc(sizes)
			rs := layoutCells(sizes, d[0], d[1])
			// 너무 작아 빈 사각형이 된 항목은 있어도, 나머지는 영역을 정확히 덮어야 함
			gridCover(t, rs, d[0], d[1])
		}
	}
}

func sortInt64Desc(s []int64) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] > s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func TestSquarifyProportional(t *testing.T) {
	sizes := []int64{600, 300, 200, 150, 100, 80, 50, 20}
	var total int64
	for _, s := range sizes {
		total += s
	}
	// 실수 좌표: 넓이가 크기에 정확히 비례
	W, H := 100.0, 40.0
	areas := make([]float64, len(sizes))
	for i, s := range sizes {
		areas[i] = float64(s) / float64(total) * W * H
	}
	frs := squarify(areas, 0, 0, W, H)
	sum := 0.0
	for i, r := range frs {
		if math.Abs(r.w()*r.h()-areas[i]) > 1e-6*W*H {
			t.Errorf("항목 %d 넓이 %.4f, 기대 %.4f", i, r.w()*r.h(), areas[i])
		}
		sum += r.w() * r.h()
	}
	if math.Abs(sum-W*H) > 1e-6 {
		t.Errorf("전체 넓이 %.4f, 기대 %.4f", sum, W*H)
	}
	// 칸 단위: 가장자리 반올림 때문에 둘레만큼의 오차까지 허용
	cw, ch := 100, 20 // 20줄 = 40 단위
	rs := layoutCells(sizes, cw, ch)
	for i, r := range rs {
		ideal := float64(sizes[i]) / float64(total) * float64(cw*ch)
		if tol := float64(r.w() + r.h() + 2); math.Abs(float64(r.area())-ideal) > tol {
			t.Errorf("항목 %d 칸 수 %d, 기대 %.1f (허용 %.0f)", i, r.area(), ideal, tol)
		}
	}
}

func TestSquarifyAspectReasonable(t *testing.T) {
	// 비슷한 크기 12개를 60x(15줄=30단위) 에 넣으면 정사각형에 가까워야 함 (한 줄로 늘어놓으면 비율이 큼)
	areas := make([]float64, 12)
	for i := range areas {
		areas[i] = 60.0 * 30 / 12
	}
	worst := 0.0
	for _, r := range squarify(areas, 0, 0, 60, 30) {
		worst = math.Max(worst, math.Max(r.w()/r.h(), r.h()/r.w()))
	}
	if worst > 2.5 {
		t.Errorf("가장 길쭉한 사각형의 비율 %.2f (2.5 이하여야 함)", worst)
	}
}

func TestLayoutZeroSizes(t *testing.T) {
	rs := layoutCells([]int64{50, 0, 30, 0}, 20, 5)
	if !rs[1].empty() || !rs[3].empty() {
		t.Errorf("크기 0 항목은 빈 사각형이어야 함: %+v", rs)
	}
	gridCover(t, rs, 20, 5)
	if rs := layoutCells([]int64{0, 0}, 20, 5); !rs[0].empty() {
		t.Errorf("전부 0 이면 모두 빈 사각형")
	}
}

func TestBuildEntriesGroupsSmall(t *testing.T) {
	root := manyKids()
	es := buildEntries(root, 68, 29)
	if len(es) == 0 {
		t.Fatal("항목이 없음")
	}
	last := es[len(es)-1]
	if last.kid != nil {
		t.Fatalf("작은 파일은 기타 묶음이어야 함: %+v", last)
	}
	if last.count < 20 || !strings.HasPrefix(last.label(), "기타 (") {
		t.Errorf("묶음 파일 수/이름이 이상함: %d %q", last.count, last.label())
	}
	if len(es) > duMaxRects+1 {
		t.Errorf("사각형이 너무 많음: %d", len(es))
	}
	var rs []cellRect
	var sum int64
	covered := 0
	for _, e := range es {
		rs = append(rs, e.rect)
		sum += e.size
		covered += e.hi - e.lo
	}
	gridCover(t, rs, 68, 29)
	if sum != root.size {
		t.Errorf("항목 합계 %d != 폴더 크기 %d", sum, root.size)
	}
	if covered != len(root.kids) {
		t.Errorf("모든 자식이 정확히 한 항목에 속해야 함: %d != %d", covered, len(root.kids))
	}
}

func TestEntryColorsDifferWhenTouching(t *testing.T) {
	es := buildEntries(manyKids(), 68, 29)
	for i := range es {
		for j := i + 1; j < len(es); j++ {
			if touching(es[i].rect, es[j].rect) && es[i].col == es[j].col {
				t.Errorf("맞닿은 사각형 %d, %d 가 같은 색", i, j)
			}
		}
	}
}

// ---- 스캐너 ----

func writeFile(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeTempTree(t *testing.T) string {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.bin"), 1000)
	writeFile(t, filepath.Join(root, "sub", "b.bin"), 2000)
	writeFile(t, filepath.Join(root, "sub", "deep", "c.bin"), 4000)
	writeFile(t, filepath.Join(root, "locked", "d.bin"), 8000)
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func find(n *dnode, name string) *dnode {
	for _, k := range n.kids {
		if k.name == name {
			return k
		}
	}
	return nil
}

func TestScanTempTreeSizes(t *testing.T) {
	root := makeTempTree(t)
	p := &scanProgress{}
	n, err := walkScan(context.Background(), root, p)
	if err != nil {
		t.Fatal(err)
	}
	if n.size != 15000 || n.files != 4 {
		t.Errorf("합계 %d (기대 15000), 파일 수 %d (기대 4)", n.size, n.files)
	}
	if s := find(n, "sub"); s == nil || s.size != 6000 || find(s, "deep").size != 4000 {
		t.Errorf("sub 크기가 다름: %+v", s)
	}
	if find(n, "empty") == nil || find(n, "empty").size != 0 {
		t.Errorf("빈 폴더가 없음")
	}
	for i := 1; i < len(n.kids); i++ {
		if n.kids[i-1].size < n.kids[i].size {
			t.Errorf("자식이 크기 내림차순이 아님")
		}
	}
	s := p.snapshot()
	if s.files != 4 || s.bytes != 15000 || s.skipped != 0 {
		t.Errorf("진행 상황이 다름: %+v", s)
	}
	if n.kids[0].parent != n {
		t.Errorf("부모 연결이 없음")
	}
}

func TestScanSkipsUnreadable(t *testing.T) {
	root := makeTempTree(t)
	old := duReadDir
	duReadDir = func(p string) ([]fs.DirEntry, error) {
		if filepath.Base(p) == "locked" {
			return nil, fs.ErrPermission
		}
		return old(p)
	}
	defer func() { duReadDir = old }()
	p := &scanProgress{}
	n, err := walkScan(context.Background(), root, p)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.snapshot().skipped; got != 1 {
		t.Errorf("건너뜀 %d, 기대 1", got)
	}
	if n.size != 7000 {
		t.Errorf("읽을 수 없는 폴더를 뺀 합계 %d, 기대 7000", n.size)
	}
	if l := find(n, "locked"); l == nil || l.size != 0 {
		t.Errorf("읽지 못한 폴더는 크기 0 으로 남아야 함: %+v", l)
	}
}

func TestScanSkipsUnreadableFileInfo(t *testing.T) {
	root := makeTempTree(t)
	old := duInfo
	duInfo = func(e fs.DirEntry) (fs.FileInfo, error) {
		if e.Name() == "c.bin" {
			return nil, fs.ErrPermission
		}
		return e.Info()
	}
	defer func() { duInfo = old }()
	p := &scanProgress{}
	n, err := walkScan(context.Background(), root, p)
	if err != nil {
		t.Fatal(err)
	}
	if p.snapshot().skipped != 1 || n.size != 11000 {
		t.Errorf("skipped=%d size=%d (기대 1, 11000)", p.snapshot().skipped, n.size)
	}
}

func TestScanSymlinkNotFollowed(t *testing.T) {
	root := makeTempTree(t)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "big.bin"), 50000)
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("심볼릭 링크를 만들 수 없음 (권한 등): %v", err)
	}
	if err := os.Symlink(root, filepath.Join(root, "sub", "loop")); err != nil {
		t.Skipf("심볼릭 링크를 만들 수 없음: %v", err)
	}
	p := &scanProgress{}
	n, err := walkScan(context.Background(), root, p)
	if err != nil {
		t.Fatal(err)
	}
	if n.size != 15000 {
		t.Errorf("링크를 따라갔음: 합계 %d, 기대 15000", n.size)
	}
	if p.snapshot().links < 2 {
		t.Errorf("링크 %d개 건너뜀, 기대 2", p.snapshot().links)
	}
}

// Windows 정션(재해석 지점): 임시 폴더 안에서만 만들고 지움. 관리자 권한이 필요 없음
func TestScanJunctionNotFollowed(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows 전용")
	}
	root := makeTempTree(t)
	link := filepath.Join(root, "sub", "loop")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, root).CombinedOutput(); err != nil {
		t.Skipf("정션을 만들 수 없음: %v %s", err, out)
	}
	defer os.Remove(link) // 정션 자체만 지움 (대상은 건드리지 않음)
	p := &scanProgress{}
	done := make(chan *dnode, 1)
	go func() {
		n, _ := walkScan(context.Background(), root, p)
		done <- n
	}()
	select {
	case n := <-done:
		if n == nil || n.size != 15000 {
			t.Errorf("정션을 따라갔음: %+v", n)
		}
		if p.snapshot().links < 1 {
			t.Errorf("정션을 건너뛴 기록이 없음")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("정션 때문에 끝나지 않음 (무한 순환?)")
	}
}

func TestScanFoldsManySmallFiles(t *testing.T) {
	root := t.TempDir()
	total := int64(0)
	for i := 0; i < duKeepFiles+50; i++ {
		writeFile(t, filepath.Join(root, fmt.Sprintf("f%03d.bin", i)), 10+i%7)
		total += int64(10 + i%7)
	}
	n, err := walkScan(context.Background(), root, &scanProgress{})
	if err != nil {
		t.Fatal(err)
	}
	if n.size != total || n.files != int64(duKeepFiles+50) {
		t.Errorf("합계 %d/%d, 파일 수 %d", n.size, total, n.files)
	}
	var other *dnode
	for _, k := range n.kids {
		if k.isOther {
			other = k
		}
	}
	if other == nil || other.files != 50 || len(n.kids) != duKeepFiles+1 {
		t.Errorf("작은 파일 묶음이 이상함: kids=%d other=%+v", len(n.kids), other)
	}
}

func TestScanNotADir(t *testing.T) {
	f := filepath.Join(t.TempDir(), "x.txt")
	writeFile(t, f, 3)
	if _, err := walkScan(context.Background(), f, &scanProgress{}); err == nil {
		t.Error("파일을 스캔하면 오류여야 함")
	}
	if _, err := walkScan(context.Background(), filepath.Join(t.TempDir(), "none"), &scanProgress{}); err == nil {
		t.Error("없는 경로는 오류여야 함")
	}
}

func TestScanCancelledContext(t *testing.T) {
	root := makeTempTree(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n, err := walkScan(ctx, root, &scanProgress{})
	if err != context.Canceled || n != nil {
		t.Errorf("취소된 스캔: n=%v err=%v", n, err)
	}
}

// ---- 스캔 흐름 (가짜 스캐너 / 임시 폴더) ----

func TestOpensHomeFromEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeFile(t, filepath.Join(home, "x.bin"), 123)
	a := newDU(t)
	if a.home != home {
		t.Fatalf("홈 폴더 %q, 기대 %q", a.home, home)
	}
	if a.st != stIdle {
		t.Fatalf("New 에서 스캔을 시작하면 안 됨")
	}
	var app App = drive(t, a, a.Init())
	d := app.(*duApp)
	if d.st != stDone || d.top == nil || d.top.size != 123 || d.top.path != home {
		t.Fatalf("스캔 결과가 다름: st=%v top=%+v", d.st, d.top)
	}
	if d.top.name != home {
		t.Errorf("뿌리 이름 %q", d.top.name)
	}
}

func TestCancelKeyStopsScan(t *testing.T) {
	started, exited := blockingScanner(t)
	a := newDU(t)
	a.Init()
	waitClosed(t, started, "스캔 시작")
	if a.st != stScan {
		t.Fatalf("스캔 중이어야 함")
	}
	a.Update(key("c"))
	waitClosed(t, exited, "c 로 취소")
	if a.st != stCancel {
		t.Errorf("취소 후 상태 %v", a.st)
	}
	assertFrame(t, "취소됨", a.c, a.View(a.c))
}

func TestEscCancelsThenHome(t *testing.T) {
	started, exited := blockingScanner(t)
	a := newDU(t)
	a.Init()
	waitClosed(t, started, "스캔 시작")
	a.Update(key("j")) // 한 번이라도 눌렀으면 첫 Esc 는 취소만 함
	_, cmd := a.Update(key("esc"))
	if cmd != nil {
		t.Errorf("첫 Esc 는 취소만 해야 함")
	}
	waitClosed(t, exited, "Esc 로 취소")
	_, cmd = a.Update(key("esc"))
	if cmd == nil {
		t.Fatal("두 번째 Esc 는 홈으로 가야 함")
	}
	if _, ok := cmd().(goHomeMsg); !ok {
		t.Error("goHomeMsg 가 아님")
	}
}

func TestEscOnUntouchedScanningAppGoesHome(t *testing.T) {
	_, exited := blockingScanner(t)
	a := newDU(t)
	a.Init()
	_, cmd := a.Update(key("esc"))
	if cmd == nil {
		t.Fatal("아무것도 안 눌렀으면 첫 Esc 로 홈에 가야 함")
	}
	if _, ok := cmd().(goHomeMsg); !ok {
		t.Error("goHomeMsg 가 아님")
	}
	waitClosed(t, exited, "Esc 후 스캔 고루틴")
}

func TestQuitKeyCancelsAndGoesHome(t *testing.T) {
	started, exited := blockingScanner(t)
	a := newDU(t)
	a.Init()
	waitClosed(t, started, "스캔 시작")
	_, cmd := a.Update(key("q"))
	if cmd == nil {
		t.Fatal("q 는 홈으로 가야 함")
	}
	if _, ok := cmd().(goHomeMsg); !ok {
		t.Error("goHomeMsg 가 아님")
	}
	waitClosed(t, exited, "q 후 스캔 고루틴")
}

func TestStaleMessagesIgnored(t *testing.T) {
	started, _ := blockingScanner(t)
	a := newDU(t)
	a.Init()
	waitClosed(t, started, "스캔 시작")
	gen := a.gen
	a.Update(appTickMsg{Gen: gen + 100, Tag: "scan"})
	if a.spin != 0 {
		t.Error("다른 세대의 틱을 처리했음")
	}
	if _, cmd := a.Update(appTickMsg{Gen: gen, Tag: "scan"}); cmd == nil || a.spin != 1 {
		t.Error("현재 세대 틱은 처리하고 다음 틱을 걸어야 함")
	}
	a.Update(scanDoneMsg{gen: gen + 100, scanResult: scanResult{root: tDir("x", tFile("a", 1))}})
	if a.st != stScan || a.top != nil {
		t.Error("다른 세대의 결과를 받아들였음")
	}
	a.Update(key("c"))
	a.Update(appTickMsg{Gen: gen, Tag: "scan"})
	a.Update(scanDoneMsg{gen: gen, scanResult: scanResult{root: tDir("x", tFile("a", 1))}})
	if a.top != nil || a.st != stCancel {
		t.Errorf("취소 뒤 도착한 결과를 받아들였음: st=%v", a.st)
	}
}

func TestRescanAndNewPathKeepsOldTreeOnFailure(t *testing.T) {
	a := ready(t, manyKids(), 80, 24)
	old := a.top
	a.Update(key("p"))
	if a.ph != dpInput {
		t.Fatal("p 로 입력 화면이 열려야 함")
	}
	bad := filepath.Join(t.TempDir(), "없는경로")
	a.input.SetValue(bad)
	_, cmd := a.Update(key("enter"))
	var app App = drive(t, a, cmd)
	d := app.(*duApp)
	if d.st != stDone || d.top != old {
		t.Fatalf("실패하면 이전 트리를 유지해야 함: st=%v", d.st)
	}
	if !strings.Contains(d.msg, "스캔 실패") || d.msgTone != ToneDanger {
		t.Errorf("실패 메시지가 없음: %q", d.msg)
	}
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		v := d.View(c)
		assertFrame(t, "실패 메시지", c, v)
		if lineCount(v) > c.H {
			t.Errorf("실패 상태 %dx%d: %d줄", c.W, c.H, lineCount(v))
		}
	}
}

func TestPathInputGetsAllKeys(t *testing.T) {
	a := ready(t, manyKids(), 80, 24)
	a.Update(key("p"))
	for _, k := range []string{"q", "v", "c", "r", "p", "h", "j", "k", "l"} {
		a.Update(key(k))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := a.input.Value(); got != "qvcrphjk" {
		t.Errorf("입력창 값 %q, 기대 qvcrphjk (마지막 l 은 Backspace 로 지워짐)", got)
	}
	if a.ph != dpInput || a.st != stDone || a.list {
		t.Errorf("입력 중 키가 새어 나감: ph=%v st=%v list=%v", a.ph, a.st, a.list)
	}
	if _, cmd := a.Update(key("esc")); cmd != nil || a.ph != dpMain {
		t.Errorf("Esc 는 입력만 닫아야 함")
	}
}

func TestSubmitPathVariants(t *testing.T) {
	old := scanTree
	var got []string
	scanTree = func(ctx context.Context, root string, p *scanProgress) (*dnode, error) {
		got = append(got, root)
		return tDir(root, tFile("a", 1)), nil
	}
	defer func() { scanTree = old }()
	for _, in := range []string{`C:`, ` "C:\" `} {
		got = nil
		a := ready(t, manyKids(), 80, 24)
		a.Update(key("p"))
		a.input.SetValue(in)
		_, cmd := a.Update(key("enter"))
		drive(t, a, cmd)
		if runtime.GOOS == "windows" && (len(got) != 1 || got[0] != `C:\`) {
			t.Errorf("입력 %q -> 스캔 경로 %v, 기대 C:\\", in, got)
		}
	}
}

// ---- 이동 ----

func TestNavigationSpatial(t *testing.T) {
	a := ready(t, manyKids(), 120, 40)
	if !a.useTree() {
		t.Fatal("120x40 에서는 트리맵이어야 함")
	}
	es := a.entries()
	if len(es) < 4 {
		t.Fatalf("항목이 너무 적음: %d", len(es))
	}
	// 0번(가장 큰 사각형)에서 출발해 각 방향 이동이 실제로 그 방향 쪽 사각형으로 가는지
	a.sel = es[0].lo
	cx := func(r cellRect) float64 { return float64(r.x0+r.x1) / 2 }
	cy := func(r cellRect) float64 { return float64(r.y0+r.y1) / 2 }
	moved := 0
	for dir, k := range []string{"up", "down", "left", "right"} {
		for start := range es {
			if es[start].rect.empty() {
				continue
			}
			a.sel = es[start].lo
			a.Update(key(k))
			i := entryOf(es, a.sel)
			if i == start {
				continue // 그 방향에 사각형이 없음
			}
			moved++
			from, to := es[start].rect, es[i].rect
			ok := false
			switch dir {
			case 0:
				ok = cy(to) < cy(from)
			case 1:
				ok = cy(to) > cy(from)
			case 2:
				ok = cx(to) < cx(from)
			case 3:
				ok = cx(to) > cx(from)
			}
			if !ok {
				t.Fatalf("%s 키: %+v -> %+v 가 그 방향이 아님", k, from, to)
			}
		}
	}
	if moved == 0 {
		t.Fatal("어느 방향으로도 이동하지 못함")
	}
	// hjkl 도 같은 동작
	a.sel = es[0].lo
	a.Update(key("l"))
	b := ready(t, manyKids(), 120, 40)
	b.sel = es[0].lo
	b.Update(key("right"))
	if a.sel != b.sel {
		t.Errorf("l 과 → 가 다르게 동작함")
	}
}

func TestNavigationReachesEverything(t *testing.T) {
	a := ready(t, manyKids(), 120, 40)
	es := a.entries()
	seen := map[int]bool{0: true}
	queue := []int{0}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for dir := 0; dir < 4; dir++ {
			if n := moveEntry(es, cur, dir); !seen[n] && !es[n].rect.empty() {
				seen[n] = true
				queue = append(queue, n)
			}
		}
	}
	vis := 0
	for _, e := range es {
		if !e.rect.empty() {
			vis++
		}
	}
	if len(seen) != vis {
		t.Errorf("방향키로 닿는 사각형 %d / 보이는 사각형 %d", len(seen), vis)
	}
}

func TestNavigationGridExactLayout(t *testing.T) {
	// 같은 크기 4개 = 2x2 배치. 오른쪽 아래로 가는 길이 위/아래/왼쪽/오른쪽에서 일관돼야 함
	es := []tmEntry{
		{rect: cellRect{0, 0, 10, 4}}, {rect: cellRect{10, 0, 20, 4}},
		{rect: cellRect{0, 4, 10, 8}}, {rect: cellRect{10, 4, 20, 8}},
	}
	want := map[[2]int]int{{0, 3}: 1, {0, 1}: 2, {1, 2}: 0, {1, 1}: 3, {3, 0}: 1, {3, 2}: 2, {2, 3}: 3, {2, 0}: 0}
	for k, w := range want {
		if got := moveEntry(es, k[0], k[1]); got != w {
			t.Errorf("항목 %d 에서 방향 %d: %d, 기대 %d", k[0], k[1], got, w)
		}
	}
	if got := moveEntry(es, 0, 2); got != 0 {
		t.Errorf("가장자리에서는 제자리여야 함: %d", got)
	}
}

func TestDescendAscend(t *testing.T) {
	root := tDir("root",
		tDir("big", tFile("x.bin", 5000), tDir("inner", tFile("y.bin", 3000))),
		tFile("note.txt", 100),
	)
	a := ready(t, root, 120, 40)
	if a.sel != 0 || root.kids[0].name != "big" {
		t.Fatalf("선택이 가장 큰 항목이어야 함")
	}
	a.Update(key("enter"))
	if a.cur.name != "big" || a.sel != 0 {
		t.Fatalf("폴더로 들어가지 못함: %s", a.cur.name)
	}
	v := plain(a.View(a.c))
	if !strings.Contains(v, filepath.Join("/root", "big")) {
		t.Errorf("경로 줄이 없음:\n%s", v)
	}
	a.Update(key("down"))
	a.Update(key("enter")) // 아래 항목(들)이 무엇이든 파일이면 이동하지 않음
	a.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if a.cur != root {
		t.Fatalf("Backspace 로 상위로 가지 못함")
	}
	if a.cur.kids[a.sel].name != "big" {
		t.Errorf("올라오면 방금 있던 폴더가 선택돼야 함")
	}
	// 맨 위에서 Backspace 는 아무 일도 없고 Esc 는 홈
	if _, cmd := a.Update(tea.KeyMsg{Type: tea.KeyBackspace}); cmd != nil {
		t.Error("맨 위 Backspace 는 홈으로 가면 안 됨")
	}
	a.Update(key("enter"))
	if _, cmd := a.Update(key("esc")); cmd != nil || a.cur != root {
		t.Error("하위 폴더의 Esc 는 상위로만 가야 함")
	}
	if _, cmd := a.Update(key("esc")); cmd == nil {
		t.Error("맨 위의 Esc 는 홈으로 가야 함")
	}
}

func TestEnterOnFileAndGroup(t *testing.T) {
	a := ready(t, manyKids(), 120, 40)
	es := a.entries()
	g := len(es) - 1
	a.sel = es[g].lo
	a.Update(key("enter"))
	if a.cur != a.top || !strings.Contains(a.msg, "기타") {
		t.Errorf("기타 묶음은 열리지 않고 안내해야 함: %q", a.msg)
	}
	a.msg = ""
	a.list = true
	a.sel = 1 // 파일
	a.Update(key("enter"))
	if a.cur != a.top || !strings.Contains(a.msg, "파일") {
		t.Errorf("파일은 열리지 않고 안내해야 함: %q", a.msg)
	}
}

func TestListNavigation(t *testing.T) {
	a := ready(t, manyKids(), 80, 24)
	a.Update(key("v"))
	if !a.list || a.useTree() {
		t.Fatal("v 로 목록이 돼야 함")
	}
	a.Update(key("down"))
	a.Update(key("j"))
	if a.sel != 2 {
		t.Errorf("목록에서 아래로 이동: %d", a.sel)
	}
	a.Update(key("up"))
	a.Update(key("k"))
	a.Update(key("k"))
	if a.sel != 0 {
		t.Errorf("목록에서 위로 이동: %d", a.sel)
	}
	a.Update(key("v"))
	if a.list {
		t.Error("v 를 다시 누르면 맵")
	}
}

// ---- 화면 ----

func TestTreemapRendersSelectionCue(t *testing.T) {
	a := ready(t, manyKids(), 120, 40)
	v := a.View(a.c)
	p := plain(v)
	if !strings.Contains(p, "▸") {
		t.Error("선택 표시(▸)가 없음")
	}
	if es := a.entries(); es[len(es)-1].kid != nil || es[len(es)-1].rect.area() < duMinCells/2 {
		t.Errorf("아주 작은 기타 묶음도 눈에 보여야 함 (바닥 %d칸, 반올림 오차 허용): %+v", duMinCells, es[len(es)-1].rect)
	}
	var ks []*dnode
	for i := 0; i < 6; i++ {
		ks = append(ks, tFile(fmt.Sprintf("big%d.bin", i), 10000))
	}
	for i := 0; i < 80; i++ {
		ks = append(ks, tFile(fmt.Sprintf("mid%d.bin", i), 120))
	}
	g := ready(t, tDir("g", ks...), 120, 40)
	if gp := plain(g.View(g.c)); !strings.Contains(gp, "기타 (80개)") {
		t.Errorf("기타 묶음 이름이 안 보임:\n%s", gp)
	}
	if !strings.Contains(p, "부모의") || !strings.Contains(p, "겉보기 크기") {
		t.Errorf("상세 줄/안내가 없음:\n%s", p)
	}
	// 선택한 사각형은 흰 배경(colorText) 이라 다른 사각형과 시각적으로도 구분됨: 켜진 색 코드가 달라야 함
	a.sel = len(a.cur.kids) - 1
	if a.View(a.c) == v {
		t.Error("선택을 옮겨도 화면이 같음")
	}
}

func TestListFallbackOnSmallWindow(t *testing.T) {
	small := ready(t, manyKids(), 56, 17)
	if small.useTree() {
		t.Fatal("56x17 에서는 목록으로 전환돼야 함")
	}
	v := plain(small.View(small.c))
	if !strings.Contains(v, "█") || !strings.Contains(v, "창이 작아 목록으로 표시함") {
		t.Errorf("목록 + 안내가 있어야 함:\n%s", v)
	}
	if _, h, ok := (&duApp{}).mapSize(testCtx(80, 20)); !ok || h != duMinMapRows {
		t.Errorf("높이 20 이 트리맵 경계여야 함: h=%d ok=%v", h, ok)
	}
	if _, _, ok := (&duApp{}).mapSize(testCtx(80, 19)); ok {
		t.Error("높이 19 는 목록이어야 함")
	}
}

func TestAllStatesRenderAlignedAndWithinHeight(t *testing.T) {
	mk := map[string]func(t *testing.T) *duApp{
		"fresh": func(t *testing.T) *duApp { return newDU(t) },
		"scanning": func(t *testing.T) *duApp {
			a := newDU(t)
			a.st = stScan
			a.scanPath = `C:\Users\아주긴한글이름의사용자\Documents\매우긴폴더\그리고또긴폴더\끝`
			a.snap = progSnap{files: 1234567, bytes: 123 << 30, skipped: 12, cur: "아주긴한글이름의최상위폴더이름입니다_abcdefghijklmnopqrstuvwxyz"}
			return a
		},
		"cancelled": func(t *testing.T) *duApp { a := newDU(t); a.st = stCancel; return a },
		"failed": func(t *testing.T) *duApp {
			a := newDU(t)
			a.st = stFail
			a.say("스캔 실패: open C:\\아주긴경로\\아주긴경로\\아주긴경로\\아주긴경로\\아주긴경로\\아주긴경로: Access is denied.", ToneDanger)
			return a
		},
		"input": func(t *testing.T) *duApp {
			a := newDU(t)
			a.Update(key("p"))
			a.input.SetValue(strings.Repeat("가나다", 20))
			return a
		},
		"treemap": func(t *testing.T) *duApp { return ready(t, manyKids(), 80, 24) },
		"list": func(t *testing.T) *duApp {
			a := ready(t, manyKids(), 80, 24)
			a.list = true
			a.sel = 30
			return a
		},
		"empty": func(t *testing.T) *duApp { return ready(t, tDir("빈폴더"), 80, 24) },
		"zero-size": func(t *testing.T) *duApp {
			return ready(t, tDir("r", tFile("a", 0), tDir("b")), 80, 24)
		},
		"skipped": func(t *testing.T) *duApp {
			a := ready(t, manyKids(), 80, 24)
			a.skipped = 123456
			return a
		},
		"single": func(t *testing.T) *duApp { return ready(t, tDir("r", tFile("a", 5)), 80, 24) },
		"deep": func(t *testing.T) *duApp {
			a := ready(t, tDir("r", tDir("d", tFile("a", 5), tFile("b", 3))), 80, 24)
			a.Update(key("enter"))
			return a
		},
		"huge-sizes": func(t *testing.T) *duApp {
			return ready(t, tDir("r", tFile("a", 1<<50), tFile("b", 1<<49), tFile("c", 7)), 80, 24)
		},
	}
	for name, mkApp := range mk {
		for _, sz := range testSizes {
			a := mkApp(t)
			c := testCtx(sz[0], sz[1])
			v := a.View(c)
			label := fmt.Sprintf("%s %dx%d", name, sz[0], sz[1])
			assertFrame(t, label, c, v)
			if n := lineCount(v); n > c.H {
				t.Errorf("%s: %d줄, 창 높이 %d 초과\n%s", label, n, c.H, plain(v))
			}
			// 선택을 끝까지 옮겨 봐도 같은 규칙을 지켜야 함
			if a.st == stDone && a.ph == dpMain {
				for _, k := range []string{"j", "j", "l", "j", "l", "down", "right"} {
					a.Update(key(k))
					v := a.View(c)
					assertFrame(t, label+" after "+k, c, v)
					if lineCount(v) > c.H {
						t.Errorf("%s after %s: 창 높이 초과", label, k)
					}
				}
			}
		}
	}
}

func TestMapRowsAreExactWidth(t *testing.T) {
	// 맵 줄은 한글 이름이 경계에 걸려도 정확히 칸 수를 채워야 함 (assertFrame 이 폭을 검사)
	for _, sz := range testSizes {
		a := ready(t, manyKids(), sz[0], sz[1])
		c := testCtx(sz[0], sz[1])
		assertFrame(t, "map", c, a.View(c))
	}
}

func TestFreshDiskAppEscGoesHome(t *testing.T) {
	a := newDU(t)
	_, cmd := a.Update(key("esc"))
	if cmd == nil {
		t.Fatal("처음 상태에서 Esc 는 홈으로 가야 함")
	}
	if _, ok := cmd().(goHomeMsg); !ok {
		t.Error("goHomeMsg 가 아님")
	}
	if diskInfo.ID != "diskusage" || diskInfo.Name != "디스크 사용량" || diskInfo.New == nil {
		t.Errorf("등록 정보: %+v", diskInfo)
	}
}

func TestNoGoroutineLeakAfterNormalScan(t *testing.T) {
	root := makeTempTree(t)
	old := scanTree
	exited := make(chan struct{})
	scanTree = func(ctx context.Context, r string, p *scanProgress) (*dnode, error) {
		defer close(exited)
		return old(ctx, r, p)
	}
	defer func() { scanTree = old }()
	a := newDU(t)
	a.home = root
	var app App = drive(t, a, a.Init())
	waitClosed(t, exited, "정상 종료")
	waitClosed(t, app.(*duApp).done, "done 채널")
	if app.(*duApp).top.size != 15000 {
		t.Errorf("합계 %d", app.(*duApp).top.size)
	}
}

// ---- 표기 ----

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0: "0 B", 1023: "1023 B", 1024: "1.0 KB", 1536: "1.5 KB",
		1 << 20: "1.0 MB", 5 << 30: "5.0 GB", 1 << 40: "1.0 TB", 3 << 50: "3072.0 TB",
	}
	for n, want := range cases {
		if got := humanSize(n); got != want {
			t.Errorf("humanSize(%d)=%q, 기대 %q", n, got, want)
		}
	}
	if got := commaInt(1234567); got != "1,234,567" {
		t.Errorf("commaInt=%q", got)
	}
	if got := commaInt(12); got != "12" {
		t.Errorf("commaInt=%q", got)
	}
}

func TestTruncLeft(t *testing.T) {
	if got := truncLeft("abc", 5); got != "abc" {
		t.Errorf("%q", got)
	}
	got := truncLeft(`C:\사용자\문서\아주긴폴더`, 12)
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "아주긴폴더") || len([]rune(got)) == 0 {
		t.Errorf("%q", got)
	}
	if w := lipgloss.Width(got); w > 12 {
		t.Errorf("폭 %d > 12: %q", w, got)
	}
	if truncLeft("abc", 0) != "" {
		t.Error("폭 0")
	}
}

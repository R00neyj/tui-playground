package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// 사용량 앱 테스트. 기록 파일은 전부 직접 만든 가짜 데이터이고, 진짜 ~/.claude, ~/.codex 는 읽지 않음.

func cl(ts time.Time, id, req, model string, in, out, cw, cr int) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"requestId":%q,"message":{"id":%q,"model":%q,"role":"assistant","content":[{"type":"text","text":"x"}],"usage":{"input_tokens":%d,"output_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d}}}`,
		ts.UTC().Format("2006-01-02T15:04:05.000Z"), req, id, model, in, out, cw, cr)
}

func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setUsageDirs(t *testing.T, claude, codex string) {
	t.Helper()
	oc, ox, oldClock := claudeUsageDir, codexUsageDir, usageClock
	claudeUsageDir = func() string { return claude }
	codexUsageDir = func() string { return codex }
	usageClock = func() time.Time { return fixedNow }
	t.Cleanup(func() { claudeUsageDir, codexUsageDir, usageClock = oc, ox, oldClock })
}

// loadApp 은 Init 이 돌려준 명령을 실행해서 결과 메시지를 앱에 먹임 (읽기가 끝난 상태로 만듦)
func loadApp(t *testing.T, a App) App {
	t.Helper()
	cmd := a.Init()
	if cmd == nil {
		t.Fatal("Init 이 읽기 명령을 돌려주지 않음")
	}
	a, _ = a.Update(cmd())
	return a
}

// ---- 파서 ----

func TestClaudeParseDedup(t *testing.T) {
	t0 := fixedNow.Add(-1 * time.Hour)
	huge := `{"type":"user","message":{"role":"user","content":"` + strings.Repeat("a", 600<<10) + `"},"output_tokens":1}`
	src := strings.Join([]string{
		cl(t0.Add(2*time.Second), "m1", "r1", "claude-sonnet-5-5", 10, 5, 100, 1000), // 같은 응답의 줄들: 출력이 커짐
		cl(t0, "m1", "r1", "claude-sonnet-5-5", 10, 1, 100, 1000),
		cl(t0.Add(time.Second), "m1", "r1", "claude-sonnet-5-5", 10, 3, 100, 1000),
		cl(t0, "m2", "r2", "claude-opus-5-5", 1, 1, 0, 0),
		cl(t0, "m3", "r3", "<synthetic>", 0, 0, 0, 0),                                                                 // 가짜 응답은 제외
		`{"type":"assistant","timestamp":"not a time","message":{"id":"m4","model":"x","usage":{"output_tokens":1}}}`, // 시각 불량
		`{"type":"user","text":"output_tokens 라는 글자만 있는 줄"}`,
		`{깨진 줄 "output_tokens"`,
		huge,
		cl(t0, "m5", "r5", "claude-haiku-5-5", 2, 2, 2, 2),
	}, "\n") // 마지막 줄은 개행 없이 끝남
	recs, err := parseClaudeUsage(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("기록 %d건, 기대 3건: %+v", len(recs), recs)
	}
	m1 := recs[0]
	if m1.Out != 5 || m1.In != 10 || m1.CacheW != 100 || m1.CacheR != 1000 || !m1.TS.Equal(t0) {
		t.Errorf("중복 합치기 결과가 다름(출력은 최대, 시각은 최소여야 함): %+v", m1)
	}
}

func TestClaudeAcrossFilesDedupAndCache(t *testing.T) {
	dir := t.TempDir()
	setUsageDirs(t, dir, filepath.Join(dir, "none"))
	t0 := fixedNow.Add(-time.Hour)
	a := filepath.Join(dir, "p1", "a.jsonl")
	b := filepath.Join(dir, "p1", "sub", "agents", "b.jsonl")
	writeLines(t, a, cl(t0, "m1", "r1", "claude-sonnet-5-5", 10, 4, 0, 0), cl(t0, "m2", "r2", "claude-sonnet-5-5", 1, 1, 0, 0))
	writeLines(t, b, cl(t0, "m1", "r1", "claude-sonnet-5-5", 10, 9, 0, 0)) // 다른 파일에 복사된 같은 응답
	writeLines(t, filepath.Join(dir, "p1", "note.txt"), "jsonl 아님")

	sum := func(d *usageData) (s usageSum) {
		for _, r := range d.recs {
			s.add(r)
		}
		return
	}
	hash := func() [32]byte {
		var all []byte
		for _, p := range []string{a, b} {
			x, _ := os.ReadFile(p)
			st, _ := os.Stat(p)
			all = append(all, x...)
			all = append(all, []byte(st.ModTime().String())...)
		}
		return sha256.Sum256(all)
	}
	h0 := hash()
	cache := newUsageCache()
	d := usageRun(claudeUsageSource, cache, &usageLoad{})
	if d.files != 2 || d.reread != 2 || d.failed != 0 || len(d.recs) != 2 {
		t.Fatalf("files=%d reread=%d failed=%d recs=%d", d.files, d.reread, d.failed, len(d.recs))
	}
	if s := sum(d); s.In != 11 || s.Out != 10 { // m1: 입력 10, 출력 max(4,9)=9 / m2: 1, 1
		t.Errorf("합계 %+v", s)
	}
	if hash() != h0 {
		t.Error("기록 파일이 읽는 동안 바뀜")
	}
	// 바뀐 게 없으면 아무것도 다시 읽지 않음
	if d2 := usageRun(claudeUsageSource, cache, &usageLoad{}); d2.reread != 0 || sum(d2) != sum(d) {
		t.Errorf("캐시가 쓰이지 않음: reread=%d", d2.reread)
	}
	// 파일 하나가 커지면 그 파일만 다시 읽음
	writeLines(t, a, cl(t0, "m1", "r1", "claude-sonnet-5-5", 10, 4, 0, 0), cl(t0, "m2", "r2", "claude-sonnet-5-5", 1, 1, 0, 0), cl(t0, "m6", "r6", "claude-sonnet-5-5", 7, 7, 0, 0))
	d3 := usageRun(claudeUsageSource, cache, &usageLoad{})
	if d3.reread != 1 || sum(d3).In != 18 {
		t.Errorf("reread=%d sum=%+v", d3.reread, sum(d3))
	}
	// 파일이 사라지면 캐시에서도 빠짐
	os.Remove(b)
	d4 := usageRun(claudeUsageSource, cache, &usageLoad{})
	if d4.files != 1 || len(cache.m) != 1 {
		t.Errorf("files=%d cache=%d", d4.files, len(cache.m))
	}
}

func codexLines(ts time.Time) []string {
	at := func(d time.Duration) string { return ts.Add(d).UTC().Format("2006-01-02T15:04:05.000Z") }
	tc := func(d time.Duration, tin, tcached, tout int) string {
		return fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":%d,"cached_input_tokens":%d,"output_tokens":%d,"reasoning_output_tokens":0,"total_tokens":%d},"last_token_usage":{"input_tokens":1,"cached_input_tokens":0,"output_tokens":1,"reasoning_output_tokens":0,"total_tokens":2}},"rate_limits":null}}`,
			at(d), tin, tcached, tout, tin+tout)
	}
	return []string{
		fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"id":"s1","cwd":"/x","cli_version":"0.1"}}`, at(0)),
		fmt.Sprintf(`{"timestamp":%q,"type":"turn_context","payload":{"model":"gpt-5-codex","cwd":"/x"}}`, at(time.Second)),
		tc(2*time.Second, 1000, 800, 50),
		tc(3*time.Second, 1000, 800, 50), // 같은 값이 다시 기록됨 -> 증가분 0
		fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{}}}`, at(4*time.Second)),
		fmt.Sprintf(`{"timestamp":%q,"type":"turn_context","payload":{"model":"gpt-5"}}`, at(5*time.Second)),
		tc(6*time.Second, 1500, 1000, 90),
		tc(7*time.Second, 100, 0, 10), // 누적이 줄어듦 -> 새 기준
	}
}

func TestCodexParse(t *testing.T) {
	src := strings.Join(codexLines(fixedNow.Add(-time.Hour)), "\n") + "\n{깨진 줄 token_count\n"
	recs, err := parseCodexUsage(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []usageRec{
		{Model: "gpt-5-codex", In: 200, Out: 50, CacheR: 800},
		{Model: "gpt-5", In: 300, Out: 40, CacheR: 200}, // 증가분: input 500 중 캐시 200, 출력 40
		{Model: "gpt-5", In: 100, Out: 10, CacheR: 0},
	}
	if len(recs) != len(want) {
		t.Fatalf("기록 %d건: %+v", len(recs), recs)
	}
	for i, w := range want {
		g := recs[i]
		if g.Model != w.Model || g.In != w.In || g.Out != w.Out || g.CacheR != w.CacheR || g.CacheW != 0 {
			t.Errorf("%d번째: %+v, 기대 %+v", i, g, w)
		}
	}
}

func TestCodexParseLastOnly(t *testing.T) {
	line := fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":10,"cached_input_tokens":4,"output_tokens":3}}}}`, fixedNow.UTC().Format(time.RFC3339))
	recs, _ := parseCodexUsage(strings.NewReader(line))
	if len(recs) != 1 || recs[0].In != 6 || recs[0].CacheR != 4 || recs[0].Out != 3 || recs[0].Model == "" {
		t.Errorf("%+v", recs)
	}
}

// ---- 집계 ----

func TestUsageReportPeriods(t *testing.T) {
	now := fixedNow // 2026-11-05(목) 14:32:07 로컬. 오늘 자정 = 00:00, 이번 주 월요일 = 11-02
	at := func(d time.Duration) time.Time { return now.Add(d) }
	today0 := time.Date(2026, 11, 5, 0, 0, 0, 0, time.Local)
	mon := time.Date(2026, 11, 2, 0, 0, 0, 0, time.Local)
	recs := []usageRec{
		{TS: at(-4 * time.Hour), Model: "a", In: 1, Out: 1},                        // 5시간, 오늘, 이번 주
		{TS: at(-6 * time.Hour), Model: "a", In: 10, Out: 10},                      // 오늘, 이번 주 (5시간 밖)
		{TS: today0.Add(-time.Minute), Model: "b", In: 100, Out: 100},              // 어제 23:59 -> 이번 주만
		{TS: mon.Add(time.Minute), Model: "b", In: 1000, Out: 1000},                // 월요일 -> 이번 주
		{TS: mon.Add(-time.Minute), Model: "b", In: 10000, Out: 10000},             // 지난주 일요일 -> 막대에만
		{TS: today0.AddDate(0, 0, -7).Add(time.Hour), Model: "b", In: 5, Out: 5},   // 7일 밖
		{TS: at(-5*time.Hour - time.Minute), Model: "c", In: 3, Out: 3, CacheR: 9}, // 5시간 바로 밖
	}
	rep := buildUsageReport(recs, now)
	cases := []struct {
		p      int
		in     int64
		models int
	}{{0, 1, 1}, {1, 1 + 10 + 3, 2}, {2, 1 + 10 + 3 + 100 + 1000, 3}}
	for _, c := range cases {
		if rep.totals[c.p].In != c.in || len(rep.rows[c.p]) != c.models {
			t.Errorf("%s: 입력 %d(기대 %d) 모델 %d(기대 %d)", usagePeriodNames[c.p], rep.totals[c.p].In, c.in, len(rep.rows[c.p]), c.models)
		}
	}
	if rep.days[0].Label != "11-05 오늘" || !rep.days[0].Today || rep.days[0].V != 2+20+6 {
		t.Errorf("오늘 막대: %+v", rep.days[0])
	}
	if rep.days[1].V != 200 || rep.days[1].Label != "11-04" {
		t.Errorf("어제 막대: %+v", rep.days[1])
	}
	if rep.days[3].V != 2000 || rep.days[4].V != 20000 || rep.days[6].Label != "10-30" {
		t.Errorf("막대: %+v", rep.days)
	}
	if r := rep.rows[2]; r[0].Model != "b" { // 모델 행은 토큰이 많은 순
		t.Errorf("정렬: %+v", r)
	}
	sun := time.Date(2026, 11, 8, 12, 0, 0, 0, time.Local) // 일요일에는 6일 전 월요일이 이번 주 시작
	if st, _ := usageStarts(sun); st[2].Day() != 2 {
		t.Errorf("일요일의 이번 주 시작: %v", st[2])
	}
}

func TestUsageFormats(t *testing.T) {
	for in, want := range map[int64]string{0: "0", 999: "999", 1000: "1,000", 21908402: "21,908,402", 5707536197: "5,707,536,197"} {
		if got := usageCommas(in); got != want {
			t.Errorf("usageCommas(%d)=%q", in, got)
		}
	}
	for in, want := range map[int64]string{0: "0", 999: "999", 1500: "1.5K", 21908402: "21.9M", 5707536197: "5.7B"} {
		if got := usageCompact(in); got != want {
			t.Errorf("usageCompact(%d)=%q", in, got)
		}
	}
	for in, want := range map[int64]string{0: "-", 100: "<1K", 390000: "390K", 611600: "612K", 11024000: "11,024K"} {
		if got := usageK(in); got != want {
			t.Errorf("usageK(%d)=%q", in, got)
		}
	}
}

// ---- 화면 ----

func sampleData(n int) *usageData {
	d := &usageData{files: 475, reread: 475, at: fixedNow.Add(-2 * time.Second), dir: "/x"}
	models := []string{"claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5", "claude-opus-5", "claude-sonnet-5", "claude-haiku-4-5-20251001", "claude-very-long-model-name-for-test", "another-model"}
	for i := 0; i < n && i < len(models); i++ {
		for day := 0; day < 9; day++ {
			d.recs = append(d.recs, usageRec{
				TS: fixedNow.Add(-time.Duration(day)*24*time.Hour - time.Duration(i)*time.Minute), Model: models[i],
				In: 150305 + int64(i), Out: 41246, CacheW: 640533, CacheR: 5707536197, // 큰 자릿수
			})
		}
	}
	return d
}

// usageMinH 는 박스가 창 높이 안에 들어오도록 보장하는 가장 낮은 창 높이 (testSizes 의 최소는 17).
// 이보다 낮으면 아래 줄이 잘릴 수 있음 (상단 3줄 + 탭 + 머리글 + 합계 + 하단 5줄 = 11줄은 줄일 수 없고 막대 3일을 더한 14줄이 바닥).
const usageMinH = 14

// assertFits 는 화면 줄 수가 창 높이를 넘지 않는지 검사함 (assertFrame 은 폭과 테두리만 봄)
func assertFits(t *testing.T, name string, c Ctx, view string) {
	t.Helper()
	if c.H < usageMinH {
		return
	}
	if h := strings.Count(view, "\n") + 1; h > c.H {
		t.Errorf("%s %dx%d: 화면 %d줄이 창 높이 %d 를 넘음", name, c.W, c.H, h, c.H)
	}
}

func TestUsageViewsAligned(t *testing.T) {
	for _, src := range []*usageSource{claudeUsageSource, codexUsageSource} {
		for _, sz := range testSizes {
			c := testCtx(sz[0], sz[1])
			for p := 0; p < usagePeriods; p++ {
				for _, n := range []int{0, 1, 3, 8} {
					a := newUsageApp(src)
					a.period = p
					assertFrame(t, src.Name+"-loading", c, a.View(c)) // 처음 읽는 중
					assertFits(t, src.Name+"-loading", c, a.View(c))
					a.ld.total.Store(475)
					a.ld.done.Store(312)
					assertFrame(t, src.Name+"-loading-progress", c, a.View(c))
					assertFits(t, src.Name+"-loading-progress", c, a.View(c))
					a.busy, a.data = false, sampleData(n)
					assertFrame(t, fmt.Sprintf("%s-data%d-p%d", src.Name, n, p), c, a.View(c))
					assertFits(t, fmt.Sprintf("%s-data%d-p%d", src.Name, n, p), c, a.View(c))
					a.busy = true // 다시 읽는 중
					assertFrame(t, src.Name+"-refresh", c, a.View(c))
					assertFits(t, src.Name+"-refresh", c, a.View(c))
					a.busy, a.data = false, &usageData{dir: "/home/someone/very/long/path/that/goes/on/and/on/and/on/.claude/projects", at: fixedNow}
					assertFrame(t, src.Name+"-empty", c, a.View(c))
					assertFits(t, src.Name+"-empty", c, a.View(c))
					a.data.err = fmt.Errorf("permission denied: 아주 긴 오류 문구가 이어지는 경우에도 박스가 깨지면 안 됨 아주 길게")
					assertFrame(t, src.Name+"-error", c, a.View(c))
					assertFits(t, src.Name+"-error", c, a.View(c))
					a.data = &usageData{files: 3, failed: 2, at: fixedNow}
					assertFrame(t, src.Name+"-failed", c, a.View(c))
					assertFits(t, src.Name+"-failed", c, a.View(c))
				}
			}
		}
	}
}

// 높이 14 ~ 60 을 모두 훑으면서 화면이 창 안에 들어오고, 높이가 넉넉하면 예전 배치(막대 7일, 모델 행 3줄 이상)를 지키는지 봄
func TestUsageHeightSweep(t *testing.T) {
	for h := usageMinH; h <= 60; h++ {
		models, days := usageHeights(h)
		if h >= 21 && days != 7 {
			t.Errorf("높이 %d: 막대 %d일 (7일이어야 함)", h, days)
		}
		if h >= 21 && models < 3 {
			t.Errorf("높이 %d: 모델 행 %d줄 (3줄 이상이어야 함)", h, models)
		}
		if days < 3 {
			t.Errorf("높이 %d: 막대 %d일 (3일 이상이어야 함)", h, days)
		}
		for _, w := range []int{56, 80, 120} {
			c := testCtx(w, h)
			for _, n := range []int{0, 1, 2, 3, 5, 8} {
				for p := 0; p < usagePeriods; p++ {
					a := newUsageApp(claudeUsageSource)
					a.period = p
					a.busy, a.data = false, sampleData(n)
					v := a.View(c)
					assertFrame(t, "sweep", c, v)
					assertFits(t, fmt.Sprintf("sweep-n%d-p%d", n, p), c, v)
				}
			}
		}
	}
}

// 작은 창(56x17)에서도 핵심 정보는 남음: 합계, 가장 많이 쓴 모델, 오늘 막대, 하단 메뉴와 상태 줄
func TestUsageSmallWindowKeepsEssentials(t *testing.T) {
	c := testCtx(56, 17)
	a := newUsageApp(claudeUsageSource)
	a.busy, a.data = false, sampleData(3)
	v := plain(a.View(c))
	for _, want := range []string{"합계", "claude-opus-5-5", "11-05 오늘", "11-04", "Esc", "홈으로", "기록 파일 475개를 읽음", "╭", "╰"} {
		if !strings.Contains(v, want) {
			t.Errorf("56x17 화면에 %q 없음:\n%s", want, v)
		}
	}
	// 모델이 아주 많으면 "그 밖 N개" 줄로 묶임
	a.data = sampleData(8)
	if v := plain(a.View(testCtx(56, 20))); !strings.Contains(v, "그 밖") {
		t.Errorf("모델이 많은데 그 밖 줄이 없음:\n%s", v)
	}
	// 읽는 중에는 진행 막대가 남음
	l := newUsageApp(codexUsageSource)
	l.ld.total.Store(475)
	l.ld.done.Store(312)
	if v := plain(l.View(c)); !strings.Contains(v, "312/475") || !strings.Contains(v, "홈으로") {
		t.Errorf("56x17 로딩 화면:\n%s", v)
	}
	// 높이가 넉넉하면(24) 예전과 같은 배치: 막대 7일이 모두 보임
	w := newUsageApp(claudeUsageSource)
	w.busy, w.data = false, sampleData(3)
	if v := plain(w.View(testCtx(80, 24))); !strings.Contains(v, "10-30") {
		t.Errorf("80x24 에서 막대 7일이 다 보이지 않음:\n%s", v)
	}
}

func TestUsageViewContent(t *testing.T) {
	a := newUsageApp(claudeUsageSource)
	a.busy, a.data = false, sampleData(3)
	c := testCtx(120, 40)
	v := plain(a.View(c))
	for _, want := range []string{"기간", "[ 오늘 ]", "최근 5시간", "이번 주", "모델", "입력", "출력", "캐시 생성", "캐시 읽기", "claude-opus-5-5", "합계", "11-05 오늘", "11-04", "10-30", "기록 파일 475개를 읽음", "마지막 갱신 14:32:05", "새로고침", "홈으로"} {
		if !strings.Contains(v, want) {
			t.Errorf("화면에 %q 없음:\n%s", want, v)
		}
	}
	if !strings.Contains(v, "150,305") { // 넓은 창에서는 천 단위 쉼표 전체 숫자
		t.Errorf("전체 숫자가 아님:\n%s", v)
	}
	// 좁은 창에서는 줄임 표기로 바뀌고 모델 이름이 남음
	n := plain(a.View(testCtx(56, 24)))
	if !strings.Contains(n, "claude-opus-5-5") || strings.Contains(n, "5,707,536,197") {
		t.Errorf("좁은 창 표:\n%s", n)
	}
	// 코덱스: 캐시 생성은 -
	x := newUsageApp(codexUsageSource)
	x.busy, x.data = false, sampleData(2)
	if cw := plain(x.View(c)); strings.Contains(cw, "640,533") {
		t.Errorf("Codex 는 캐시 생성을 - 로 보여야 함:\n%s", cw)
	}
	// 읽는 중에도 탭 이동은 되고 안내 문구와 진행 상태가 보임
	l := newUsageApp(claudeUsageSource)
	l.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if l.period != 0 {
		t.Errorf("읽는 중 탭 이동 실패: %d", l.period)
	}
	if lv := plain(l.View(c)); !strings.Contains(lv, "읽는 동안에도 ←→ 와 Esc 는 쓸 수 있음") {
		t.Errorf("로딩 화면:\n%s", lv)
	}
	l.ld.total.Store(475)
	l.ld.done.Store(312)
	if lv := plain(l.View(c)); !strings.Contains(lv, "기록 읽는 중…") || !strings.Contains(lv, "312/475") {
		t.Errorf("진행 상태:\n%s", lv)
	}
}

func TestUsageEmptyAndKeys(t *testing.T) {
	dir := t.TempDir()
	setUsageDirs(t, filepath.Join(dir, "claude-none"), filepath.Join(dir, "codex-none"))
	for _, tc := range []struct {
		info  AppInfo
		title string
	}{{claudeUsageInfo, "Claude Code 기록을 찾을 수 없음"}, {codexUsageInfo, "Codex 기록을 찾을 수 없음"}} {
		a := loadApp(t, tc.info.New(&Env{}))
		for _, sz := range testSizes {
			c := testCtx(sz[0], sz[1])
			v := a.View(c)
			assertFrame(t, tc.info.ID+"-empty", c, v)
			if !strings.Contains(plain(v), tc.title) || !strings.Contains(plain(v), "기록 파일 0개") || !strings.Contains(plain(v), "다시 찾기") {
				t.Errorf("%s 빈 화면:\n%s", tc.info.ID, plain(v))
			}
		}
		// → 는 아무 일도 안 하고, r 은 다시 찾고, Esc 는 홈
		if _, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRight}); cmd != nil {
			t.Error("빈 화면에서 → 가 명령을 만듦")
		}
		a2, cmd := a.Update(key("r"))
		if cmd == nil {
			t.Fatal("r 로 다시 찾지 않음")
		}
		if !strings.Contains(plain(a2.View(testCtx(80, 24))), "다시 찾는 중") {
			t.Error("다시 찾는 중 표시가 없음")
		}
		a3, _ := a2.Update(cmd())
		if _, cmd := a3.Update(key("esc")); cmd == nil {
			t.Error("Esc 가 홈으로 가지 않음")
		} else if _, ok := cmd().(goHomeMsg); !ok {
			t.Error("Esc 결과가 goHomeMsg 가 아님")
		}
	}
}

func TestUsageEscWhileLoadingAndStaleGen(t *testing.T) {
	a := newUsageApp(claudeUsageSource)
	if _, cmd := a.Update(key("esc")); cmd == nil {
		t.Fatal("읽는 중 Esc 가 안 먹음")
	} else if _, ok := cmd().(goHomeMsg); !ok {
		t.Fatal("goHomeMsg 아님")
	}
	if _, cmd := a.Update(key("r")); cmd != nil { // 읽는 중에는 r 무시
		t.Error("읽는 중에 r 이 또 읽기를 시작함")
	}
	a.Update(usageLoadedMsg{gen: a.gen + 1, data: &usageData{files: 1}}) // 다른 gen 의 결과는 무시
	if a.data != nil || !a.busy {
		t.Error("다른 gen 의 결과를 받아들임")
	}
	a.Update(usageLoadedMsg{gen: a.gen, data: &usageData{files: 1, at: fixedNow}})
	if a.data == nil || a.busy {
		t.Error("자기 gen 의 결과를 못 받음")
	}
	for i := 0; i < 5; i++ { // 탭 이동은 양 끝에서 멈춤
		a.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	if a.period != usagePeriods-1 {
		t.Errorf("period=%d", a.period)
	}
}

func TestClaudeAppEndToEnd(t *testing.T) {
	dir := t.TempDir()
	setUsageDirs(t, dir, filepath.Join(dir, "none"))
	t0 := fixedNow.Add(-30 * time.Minute)
	writeLines(t, filepath.Join(dir, "proj", "s.jsonl"),
		cl(t0, "m1", "r1", "claude-sonnet-5-5", 1200, 300, 4000, 90000),
		cl(t0.Add(time.Second), "m1", "r1", "claude-sonnet-5-5", 1200, 800, 4000, 90000),
		cl(t0, "m2", "r2", "claude-opus-5-5", 5, 6, 7, 8),
	)
	a := loadApp(t, claudeUsageInfo.New(&Env{}))
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		v := a.View(c)
		assertFrame(t, "claude-e2e", c, v)
		p := plain(v)
		for _, want := range []string{"claude-sonnet-5-5", "claude-opus-5-5", "합계"} {
			if !strings.Contains(p, want) {
				t.Errorf("%dx%d: %q 없음\n%s", sz[0], sz[1], want, p)
			}
		}
	}
	p := plain(a.View(testCtx(120, 40)))
	for _, want := range []string{"1,205", "806", "4,007", "90,008"} { // 합계: 입력 1200+5, 출력 800+6 (중복은 큰 값), 캐시 생성 4000+7, 캐시 읽기 90000+8
		if !strings.Contains(p, want) {
			t.Errorf("합계 %q 없음\n%s", want, p)
		}
	}
	a2, cmd := a.Update(key("r")) // 바뀐 게 없으니 0개를 다시 읽음
	a2, _ = a2.Update(cmd())
	if p := plain(a2.View(testCtx(120, 40))); !strings.Contains(p, "중 0개를 다시 읽음") {
		t.Errorf("새로고침 문구:\n%s", p)
	}
}

func TestCodexAppEndToEnd(t *testing.T) {
	dir := t.TempDir()
	setUsageDirs(t, filepath.Join(dir, "none"), dir)
	writeLines(t, filepath.Join(dir, "2026", "11", "05", "rollout-2026-11-05T13-00-00-abc.jsonl"), codexLines(fixedNow.Add(-time.Hour))...)
	a := loadApp(t, codexUsageInfo.New(&Env{}))
	for _, sz := range testSizes {
		c := testCtx(sz[0], sz[1])
		v := a.View(c)
		assertFrame(t, "codex-e2e", c, v)
		if p := plain(v); !strings.Contains(p, "gpt-5-codex") || !strings.Contains(p, "합계") {
			t.Errorf("%dx%d:\n%s", sz[0], sz[1], p)
		}
	}
	p := plain(a.View(testCtx(120, 40)))
	if !strings.Contains(p, "기록 파일 1개를 읽음") || !strings.Contains(p, "gpt-5 ") {
		t.Errorf("\n%s", p)
	}
}

func TestUsageEachLineLongLines(t *testing.T) {
	long := strings.Repeat("x", usageMaxLine/8) // 버퍼(256KB)보다 훨씬 긴 줄
	in := "a\n" + long + "\nb\n\nc"
	var got []string
	usageEachLine(bytes.NewReader([]byte(in)), func(b []byte) { got = append(got, strings.TrimRight(string(b), "\n")) })
	if len(got) != 5 || got[0] != "a" || got[1] != long || got[2] != "b" || got[3] != "" || got[4] != "c" {
		t.Errorf("줄 %d개", len(got))
	}
}

package main

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// 시스템 모니터 (F-07). CPU·메모리·디스크 사용률과 가동 시간을 주기마다 읽어서 보여줌.
// 읽기는 UI 를 막지 않게 tea.Cmd(고루틴)에서 하고 결과를 sysmonSampleMsg 로 받음.

var sysmonInfo = AppInfo{
	ID:   "sysmon",
	Name: "시스템 모니터",
	Desc: "CPU·메모리·디스크 실시간",
	New:  newSysmonApp,
}

const (
	sysmonHistory = 60   // 추이에 남기는 최근 측정 횟수
	sysmonWarnAt  = 0.70 // 이 비율 이상이면 막대를 color.warn 으로 그림
)

// 갱신 주기 단계 (+ 는 다음 단계=더 느리게, - 는 이전 단계=더 빠르게)
var sysmonIntervals = []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 5 * time.Second}

const sysmonDefaultIv = 1 // sysmonIntervals 의 인덱스 (1초)

// ---- 읽기 (테스트에서 sysmonRead 를 바꿔 끼움) ----

type sysmonDisk struct {
	Label       string
	Used, Total uint64
}

// sysmonSample 은 읽은 결과. 항목마다 OK 표시가 따로 있어서 일부만 실패해도 나머지는 그대로 씀.
type sysmonSample struct {
	CPU      float64 // 0~100
	CPUOK    bool
	Cores    int
	MemUsed  uint64
	MemTotal uint64
	MemOK    bool
	Disks    []sysmonDisk
	DisksOK  bool
	Uptime   uint64 // 초
	UptimeOK bool
}

// sysmonItem 은 따로 읽는 항목. 항목마다 고루틴과 시간 제한이 따로라서 한 항목이 멈춰도 나머지는 계속 갱신됨.
type sysmonItem int

const (
	sysmonCPU sysmonItem = iota
	sysmonMem
	sysmonDrives
	sysmonUp
	sysmonItemN
)

var sysmonItemName = [sysmonItemN]string{"CPU", "메모리", "디스크", "가동 시간"}

var (
	sysmonRead      = sysmonReadReal         // 한 항목만 읽어서 그 항목의 필드만 채운 sysmonSample 을 돌려줌
	sysmonCPUWindow = 300 * time.Millisecond // CPU 사용률을 재는 구간
	sysmonTimeout   = 5 * time.Second        // 항목 하나의 읽기가 이 시간을 넘기면 그 항목만 실패로 봄 (끊긴 네트워크 드라이브 등)
)

func sysmonReadReal(item sysmonItem) sysmonSample {
	var s sysmonSample
	switch item {
	case sysmonCPU:
		if p, err := cpu.Percent(sysmonCPUWindow, false); err == nil && len(p) > 0 {
			s.CPU, s.CPUOK = p[0], true
		}
		if n, err := cpu.Counts(true); err == nil {
			s.Cores = n
		}
	case sysmonMem:
		if m, err := mem.VirtualMemory(); err == nil && m.Total > 0 {
			s.MemUsed, s.MemTotal, s.MemOK = m.Used, m.Total, true
		}
	case sysmonDrives:
		s.Disks, s.DisksOK = sysmonReadDisks()
	case sysmonUp:
		if up, err := host.Uptime(); err == nil {
			s.Uptime, s.UptimeOK = up, true
		}
	}
	return s
}

// sysmonReadDisks 는 드라이브(마운트)별 사용량. 미디어 없는 CD 드라이브처럼 개별로 못 읽는 것은 건너뛰고,
// 목록 자체를 못 얻거나 하나도 못 읽으면 실패로 봄.
func sysmonReadDisks() ([]sysmonDisk, bool) {
	parts, err := disk.Partitions(false)
	if err != nil || len(parts) == 0 {
		return nil, false
	}
	var out []sysmonDisk
	seen := map[string]bool{}
	for _, p := range parts {
		if seen[p.Device] || p.Fstype == "squashfs" || strings.HasPrefix(p.Mountpoint, "/System/Volumes/") && p.Mountpoint != "/System/Volumes/Data" {
			continue
		}
		u, err := disk.Usage(p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		seen[p.Device] = true
		out = append(out, sysmonDisk{Label: strings.TrimRight(p.Mountpoint, `\`), Used: u.Used, Total: u.Total})
	}
	return out, len(out) > 0
}

// sysmonItemCmd 는 한 항목의 읽기를 고루틴에서 돌리고 결과를 메시지로 돌려줌. 패닉이나 시간 초과는 그 항목만 "실패"로 바꿔서 앱이 멈추지 않게 함.
// running 은 읽기 고루틴이 끝나면 내려감. 시간 초과 뒤에도 멈춘 고루틴이 남아 있는 동안은 올라가 있어서 새 읽기를 시작하지 않음 (고루틴이 쌓이지 않게).
func sysmonItemCmd(gen int, item sysmonItem, running *atomic.Bool) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan sysmonSample, 1)
		go func() {
			var s sysmonSample
			func() {
				defer func() { _ = recover() }()
				s = sysmonRead(item)
			}()
			running.Store(false)
			ch <- s
		}()
		select {
		case s := <-ch:
			return sysmonSampleMsg{Gen: gen, Item: item, S: s}
		case <-time.After(sysmonTimeout):
			return sysmonSampleMsg{Gen: gen, Item: item, S: sysmonSample{}}
		}
	}
}

// sysmonSampleMsg 는 한 항목의 읽기 결과. S 에는 Item 에 해당하는 필드만 채워져 있음.
type sysmonSampleMsg struct {
	Gen  int
	Item sysmonItem
	S    sysmonSample
}

// ---- 앱 ----

type sysmonApp struct {
	gen    int
	chain  int // 틱 사슬 번호. 일시정지·재개·주기 변경 때 올려서 옛 틱을 무시함
	iv     int // sysmonIntervals 인덱스
	paused bool
	// 항목별 읽기 고루틴이 아직 도는 중인지. 돌고 있으면 그 항목은 다음 틱에 새로 읽지 않음
	running [sysmonItemN]atomic.Bool
	got     [sysmonItemN]bool // 항목별로 첫 결과(성공이든 실패든)를 받았는지
	s       sysmonSample
	cpuH    []float64
	memH    []float64
}

func newSysmonApp(env *Env) App {
	return &sysmonApp{gen: newGen(), iv: sysmonDefaultIv}
}

func (a *sysmonApp) tag() string { return fmt.Sprintf("sysmon%d", a.chain) }

func (a *sysmonApp) tick() tea.Cmd { return tickCmd(sysmonIntervals[a.iv], a.gen, a.tag()) }

// sample 은 읽기가 진행 중이지 않은 항목마다 읽기 명령을 만듦
func (a *sysmonApp) sample() tea.Cmd {
	var cmds []tea.Cmd
	for i := range sysmonItemN {
		if a.running[i].Load() {
			continue
		}
		a.running[i].Store(true)
		cmds = append(cmds, sysmonItemCmd(a.gen, i, &a.running[i]))
	}
	return tea.Batch(cmds...)
}

func (a *sysmonApp) Init() tea.Cmd { return tea.Batch(a.sample(), a.tick()) }

func (a *sysmonApp) Update(msg tea.Msg) (App, tea.Cmd) {
	switch msg := msg.(type) {
	case appTickMsg:
		if msg.Gen != a.gen || msg.Tag != a.tag() || a.paused {
			return a, nil
		}
		return a, tea.Batch(a.sample(), a.tick())

	case sysmonSampleMsg:
		if msg.Gen != a.gen {
			return a, nil
		}
		if a.paused { // 일시정지 중에 도착한 결과는 버려서 값이 고정되게 함
			return a, nil
		}
		a.applyItem(msg.Item, msg.S)

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return a, goHome()
		case "p":
			a.paused = !a.paused
			a.chain++
			if a.paused {
				return a, nil
			}
			return a, tea.Batch(a.sample(), a.tick())
		case "+", "=":
			return a, a.setIv(a.iv + 1)
		case "-", "_":
			return a, a.setIv(a.iv - 1)
		}
	}
	return a, nil
}

// setIv 는 갱신 주기 단계를 옮기고, 실행 중이면 새 주기로 틱을 다시 검
func (a *sysmonApp) setIv(i int) tea.Cmd {
	i = max(0, min(len(sysmonIntervals)-1, i))
	if i == a.iv {
		return nil
	}
	a.iv = i
	a.chain++
	if a.paused {
		return nil
	}
	return a.tick()
}

// applyItem 은 한 항목의 읽은 결과를 반영함. 다른 항목의 값은 건드리지 않음.
// 실패한 항목은 추이에 값을 더하지 않고 화면에서 "-" 로 보임.
func (a *sysmonApp) applyItem(item sysmonItem, s sysmonSample) {
	if item < 0 || item >= sysmonItemN {
		return
	}
	a.got[item] = true
	switch item {
	case sysmonCPU:
		a.s.CPU, a.s.CPUOK, a.s.Cores = s.CPU, s.CPUOK, s.Cores
		if s.CPUOK {
			a.cpuH = sysmonPush(a.cpuH, s.CPU)
		}
	case sysmonMem:
		a.s.MemUsed, a.s.MemTotal, a.s.MemOK = s.MemUsed, s.MemTotal, s.MemOK
		if s.MemOK {
			a.memH = sysmonPush(a.memH, sysmonPct(s.MemUsed, s.MemTotal)*100)
		}
	case sysmonDrives:
		a.s.Disks, a.s.DisksOK = s.Disks, s.DisksOK
	case sysmonUp:
		a.s.Uptime, a.s.UptimeOK = s.Uptime, s.UptimeOK
	}
}

// apply 는 모든 항목이 담긴 결과를 한꺼번에 반영함 (테스트용)
func (a *sysmonApp) apply(s sysmonSample) {
	for i := range sysmonItemN {
		a.applyItem(i, s)
	}
}

func sysmonPush(h []float64, v float64) []float64 {
	h = append(h, v)
	if len(h) > sysmonHistory {
		h = h[len(h)-sysmonHistory:]
	}
	return h
}

func sysmonPct(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total)
}

// ---- 화면 ----

func sysmonColor(p float64) lipgloss.Color {
	if p >= sysmonWarnAt {
		return toneColor(ToneWarn)
	}
	return colorAccent
}

func sysmonGB(b uint64) float64 { return float64(b) / (1 << 30) }

func sysmonUptime(sec uint64) string {
	d, h, m := sec/86400, sec%86400/3600, sec%3600/60
	switch {
	case d > 0:
		return fmt.Sprintf("%d일 %d시간 %d분", d, h, m)
	case h > 0:
		return fmt.Sprintf("%d시간 %d분", h, m)
	}
	return fmt.Sprintf("%d분", m)
}

func sysmonIvText(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.1f초", d.Seconds())
	}
	return fmt.Sprintf("%d초", int(d.Seconds()))
}

// sysmonMenu 는 `[p] 일시정지    [+][-] 갱신 주기 (1초)    [Esc] 홈으로` 한 줄.
// 폭이 모자라면 간격과 문구를 줄여 가며 맞춤 (주기 안내는 끝까지 남김).
func sysmonMenu(f *Frame, paused bool, iv time.Duration) {
	pl := "일시정지"
	if paused {
		pl = "재개"
	}
	t := sysmonIvText(iv)
	build := func(gap int, ivLabel string) []seg {
		sep := sg(sp(gap), colorText)
		key := func(k string) []seg {
			return []seg{sg("[", colorTextMuted), sg(k, colorAccent), sg("]", colorTextMuted)}
		}
		var segs []seg
		segs = append(segs, key("p")...)
		segs = append(segs, sg(" "+pl, colorText), sep)
		segs = append(segs, key("+")...)
		segs = append(segs, key("-")...)
		segs = append(segs, sg(" "+ivLabel, colorText), sep)
		segs = append(segs, key("Esc")...)
		return append(segs, sg(" 홈으로", colorText))
	}
	var segs []seg
	for _, v := range []struct {
		gap   int
		label string
	}{{4, "갱신 주기 (" + t + ")"}, {2, "갱신 주기 (" + t + ")"}, {2, "주기 (" + t + ")"}, {2, "주기 " + t}, {1, t}} {
		segs = build(v.gap, v.label)
		if segsWidth(segs) <= f.In()-4 {
			break
		}
	}
	f.Row(segs...)
}

// message 는 하단 한 줄 안내와 그 색 (일시정지 > 읽기 실패 > 부하 높음 > 첫 읽기 대기 순)
func (a *sysmonApp) message(in int) (string, Tone) {
	if a.paused {
		return "일시정지됨 — 값이 고정돼 있음", ToneInfo
	}
	ok := [sysmonItemN]bool{a.s.CPUOK, a.s.MemOK, a.s.DisksOK, a.s.UptimeOK}
	var bad []string
	pending := false
	for i := range sysmonItemN {
		switch {
		case !a.got[i]:
			pending = true
		case !ok[i]:
			bad = append(bad, sysmonItemName[i])
		}
	}
	if len(bad) > 0 {
		who := strings.Join(bad, "·")
		txt := who + " 값을 읽지 못함. 다음 갱신 때 다시 시도함"
		if lipgloss.Width(txt) > in-4 {
			txt = who + " 값을 읽지 못함"
		}
		return truncW(txt, in-4), ToneDanger
	}
	var hi []string
	if a.s.CPUOK && a.s.CPU/100 >= sysmonWarnAt {
		hi = append(hi, "CPU")
	}
	if a.s.MemOK && sysmonPct(a.s.MemUsed, a.s.MemTotal) >= sysmonWarnAt {
		hi = append(hi, "메모리")
	}
	if a.s.DisksOK {
		var ds []string
		for _, d := range a.s.Disks {
			if sysmonPct(d.Used, d.Total) >= sysmonWarnAt {
				ds = append(ds, d.Label)
			}
		}
		if len(ds) > 0 {
			hi = append(hi, "디스크("+strings.Join(ds, ",")+")")
		}
	}
	if len(hi) > 0 {
		return truncW(strings.Join(hi, "·")+" 사용률이 높음", in-4), ToneInfo
	}
	if pending {
		return "처음 값을 읽는 중", ToneInfo
	}
	return "", ToneInfo
}

// sysmonDiskVal 은 디스크 줄의 값 칸. 경고 구간(70% 이상)이면 색 말고도 글자로 보이게 끝에 " !" 를 붙임.
func sysmonDiskVal(d sysmonDisk) string {
	v := fmt.Sprintf("%.0f / %.0f GB", sysmonGB(d.Used), sysmonGB(d.Total))
	if sysmonPct(d.Used, d.Total) < sysmonWarnAt {
		return v
	}
	return truncW(v, meterValW-2) + " !"
}

func (a *sysmonApp) View(c Ctx) string {
	f := NewFrame(c)
	compact := c.H > 0 && c.H < 20 // 낮은 창에서는 빈 줄을 빼서 드라이브 줄 수를 확보함
	gap := func() {
		if !compact {
			f.Blank()
		}
	}
	s := a.s

	gap()
	cpuP, cpuVal := -1.0, ""
	if s.CPUOK {
		cpuP = clamp01(s.CPU / 100)
		if s.Cores > 0 {
			cpuVal = fmt.Sprintf("%d 코어", s.Cores)
		}
	}
	f.Meter("CPU", cpuP, cpuVal, sysmonColor(cpuP))
	f.Spark("추이", a.cpuH, 100, colorAccent)

	memP, memVal := -1.0, ""
	if s.MemOK {
		memP = sysmonPct(s.MemUsed, s.MemTotal)
		memVal = fmt.Sprintf("%.1f / %.1f GB", sysmonGB(s.MemUsed), sysmonGB(s.MemTotal))
	}
	f.Meter("메모리", memP, memVal, sysmonColor(memP))
	f.Spark("추이", a.memH, 100, colorAccent)

	gap()
	if s.DisksOK {
		// 위 고정 줄을 뺀 나머지를 드라이브 줄로 씀. 넘치면 마지막 줄을 "외 N개" 안내로 바꿈
		room := 8
		if c.H > 0 {
			room = max(1, c.H-16)
			if compact {
				room = max(1, c.H-13)
			}
		}
		show := s.Disks
		if len(show) > room {
			show = show[:max(0, room-1)]
		}
		for _, d := range show {
			p := sysmonPct(d.Used, d.Total)
			f.Meter(truncW(d.Label, 10), p, sysmonDiskVal(d), sysmonColor(p))
		}
		if len(show) < len(s.Disks) {
			f.Row(sg(fmt.Sprintf("외 %d개 드라이브", len(s.Disks)-len(show)), colorTextMuted))
		}
	} else {
		f.Meter("디스크", -1, "", colorAccent)
	}

	gap()
	up := "-"
	if s.UptimeOK {
		up = sysmonUptime(s.Uptime)
	}
	f.Row(sg(padR("가동 시간", 10)+" ", colorTextMuted), sg(up, colorText))

	f.Rule()
	sysmonMenu(f, a.paused, sysmonIntervals[a.iv])
	f.Rule()
	txt, tone := a.message(f.In())
	return f.Message(txt, tone).String()
}

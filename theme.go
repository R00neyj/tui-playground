package main

import "github.com/charmbracelet/lipgloss"

// 앱 이름 (DESIGN.md 헤더, 인트로 부제목, 창 제목)
const appName = "터미널 플레이그라운드"

// DESIGN.md > Tokens > Color. 값은 ANSI 16색 번호이고 실제 색은 터미널 테마를 따름.
// 화면 코드에서 색 번호를 직접 쓰지 말고 이 이름만 쓸 것.
var (
	colorText      = lipgloss.Color("15") // color.text        본문, 값
	colorTextMuted = lipgloss.Color("7")  // color.text-muted  도움말, 보조 설명, 비활성
	colorTextFaint = lipgloss.Color("8")  // color.text-faint  테두리, 구분선, 막대의 빈 칸 (글자에는 쓰지 않음)
	colorAccent    = lipgloss.Color("14") // color.accent      제목, 키 번호, 입력 프롬프트
	colorOk        = lipgloss.Color("10") // color.ok          성공, 여유 있음
	colorWarn      = lipgloss.Color("11") // color.warn        경고
	colorDanger    = lipgloss.Color("9")  // color.danger      오류, 종료 임박
	colorOnState   = lipgloss.Color("0")  // color.on-state    상태색 배경 위 글자

	// 인트로 로고 그라데이션 전용 (DESIGN.md Deliberate exceptions)
	colorIntroBlue     = lipgloss.Color("12")
	colorIntroDarkCyan = lipgloss.Color("6")
)

// Tone 은 메시지·배지·측정값의 의미 단계. 색은 toneColor 로만 바꿈.
type Tone int

const (
	ToneInfo   Tone = iota // 보조 (color.text-muted)
	ToneOk                 // 성공 (color.ok)
	ToneWarn               // 경고 (color.warn)
	ToneDanger             // 오류 (color.danger)
	ToneAccent             // 강조 (color.accent)
)

func toneColor(t Tone) lipgloss.Color {
	switch t {
	case ToneOk:
		return colorOk
	case ToneWarn:
		return colorWarn
	case ToneDanger:
		return colorDanger
	case ToneAccent:
		return colorAccent
	}
	return colorTextMuted
}

// 디스크 사용량 트리맵 전용 범주색 (배경으로 쓰고 위에 colorOnState 글자를 올림)
var (
	colorViz1 = lipgloss.Color("12")
	colorViz2 = lipgloss.Color("13")
	colorViz3 = lipgloss.Color("14")
	colorViz4 = lipgloss.Color("10")
	colorViz5 = lipgloss.Color("11")
	colorViz6 = lipgloss.Color("6")
)

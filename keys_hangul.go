package main

import tea "github.com/charmbracelet/bubbletea"

// 한글 입력 상태에서 누른 키를 같은 자리의 영문 키로 바꿔 줌 (두벌식 자판 기준).
// 예: 한글 상태에서 a 를 누르면 터미널은 "ㅁ" 을 보내므로, 단축키를 쓰는 앱(게임·피아노 등)이 반응하지 않음.
// 입력기가 자모를 글자로 조합해서 보내면("마") 초성·중성·종성으로 풀어서 키 여러 개("a", "k")로 돌려줌.
// 글자를 입력받는 화면(점심 메뉴 추가, 웹 브라우저 입력 등)에는 쓰지 않음.

var jamoKey = map[rune]string{
	'ㅂ': "q", 'ㅈ': "w", 'ㄷ': "e", 'ㄱ': "r", 'ㅅ': "t", 'ㅛ': "y", 'ㅕ': "u", 'ㅑ': "i", 'ㅐ': "o", 'ㅔ': "p",
	'ㅁ': "a", 'ㄴ': "s", 'ㅇ': "d", 'ㄹ': "f", 'ㅎ': "g", 'ㅗ': "h", 'ㅓ': "j", 'ㅏ': "k", 'ㅣ': "l",
	'ㅋ': "z", 'ㅌ': "x", 'ㅊ': "c", 'ㅍ': "v", 'ㅠ': "b", 'ㅜ': "n", 'ㅡ': "m",
	'ㅃ': "Q", 'ㅉ': "W", 'ㄸ': "E", 'ㄲ': "R", 'ㅆ': "T", 'ㅒ': "O", 'ㅖ': "P",
	// 겹모음·겹받침은 두 키로 풂
	'ㅘ': "hk", 'ㅙ': "ho", 'ㅚ': "hl", 'ㅝ': "nj", 'ㅞ': "np", 'ㅟ': "nl", 'ㅢ': "ml",
	'ㄳ': "rt", 'ㄵ': "sw", 'ㄶ': "sg", 'ㄺ': "fr", 'ㄻ': "fa", 'ㄼ': "fq", 'ㄽ': "ft", 'ㄾ': "fx", 'ㄿ': "fv", 'ㅀ': "fg", 'ㅄ': "qt",
}

var (
	hangulInitial = []rune("ㄱㄲㄴㄷㄸㄹㅁㅂㅃㅅㅆㅇㅈㅉㅊㅋㅌㅍㅎ")
	hangulMedial  = []rune("ㅏㅐㅑㅒㅓㅔㅕㅖㅗㅘㅙㅚㅛㅜㅝㅞㅟㅠㅡㅢㅣ")
	hangulFinal   = append([]rune{0}, []rune("ㄱㄲㄳㄴㄵㄶㄷㄹㄺㄻㄼㄽㄾㄿㅀㅁㅂㅄㅅㅆㅇㅈㅊㅋㅌㅍㅎ")...)
)

// hangulKeys 는 키 입력에 한글이 들어 있으면 영문 키 목록과 true 를 돌려줌. 한글이 없으면 false
func hangulKeys(k tea.KeyMsg) ([]string, bool) {
	if k.Type != tea.KeyRunes {
		return nil, false
	}
	var out []string
	found := false
	add := func(r rune) {
		for _, c := range jamoKey[r] {
			out = append(out, string(c))
		}
	}
	for _, r := range k.Runes {
		switch {
		case r >= 0xAC00 && r <= 0xD7A3: // 조합된 글자: 초성·중성·종성으로 풂
			found = true
			s := int(r - 0xAC00)
			add(hangulInitial[s/588])
			add(hangulMedial[s%588/28])
			if f := hangulFinal[s%28]; f != 0 {
				add(f)
			}
		case jamoKey[r] != "":
			found = true
			add(r)
		default:
			out = append(out, string(r))
		}
	}
	return out, found
}

func runeKey(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// forwardHangul 은 한글 키면 영문 키로 바꿔 update 에 하나씩 넘기고 true 를 돌려줌. 앱의 Update 맨 앞에서 씀
func forwardHangul(msg tea.Msg, update func(tea.Msg) tea.Cmd) (tea.Cmd, bool) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	ks, ok := hangulKeys(k)
	if !ok {
		return nil, false
	}
	var cmds []tea.Cmd
	for _, s := range ks {
		cmds = append(cmds, update(runeKey(s)))
	}
	return tea.Batch(cmds...), true
}

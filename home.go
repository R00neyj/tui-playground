package main

import "fmt"

// 홈 메뉴 (F-01). 앱 목록은 registry 에서 옴.

const homeNameW = 16

func (m model) viewHome(c Ctx) string {
	f := NewFrame(c).Blank()
	hint := ""
	if len(registry) == 0 {
		f.Row(sg("등록된 앱 없음", colorTextMuted))
		f.Row(sg("앱 파일을 추가하고 등록 목록에 한 줄을 넣으면 여기에 나타남", colorTextMuted))
	}
	for i, info := range registry {
		selected := i == m.sel
		mark, key := " ", "   "
		if selected {
			mark = "▸"
		}
		if i < 9 {
			key = fmt.Sprintf("[%d]", i+1)
		}
		nameSeg := sg(padR(info.Name, homeNameW), colorText)
		if selected {
			nameSeg = sgBold(padR(info.Name, homeNameW), colorAccent)
		}

		var right []seg
		if info.Status != nil {
			st := info.Status(m.env, c.Now)
			switch {
			case st.Badge != "":
				right = append(right, Badge(st.Badge, st.Tone), sg(" "+st.Text, colorText))
				if hint == "" {
					hint = st.Hint
				}
			case st.Text != "":
				right = append(right, sg(st.Text, toneColor(st.Tone)))
			}
		}
		descW := max(0, f.In()-4-2-len(key)-1-homeNameW-segsWidth(right)-2)
		left := []seg{sg(mark+" ", colorAccent), sg(key+" ", colorTextMuted), nameSeg, sg(truncW(info.Desc, descW), colorTextMuted)}
		f.RowLR(left, right)
	}
	f.Blank().Rule()
	n := len(registry)
	pick := "1-9"
	if n < 9 {
		pick = fmt.Sprintf("1-%d", max(n, 1))
	}
	f.MenuInline([2]string{"↑↓", "선택"}, [2]string{pick, "바로 열기"}, [2]string{"Enter", "실행"}, [2]string{"Esc", "종료"})
	f.Rule().Message(hint, ToneInfo)
	return f.String()
}

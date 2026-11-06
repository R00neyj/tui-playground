package main

import (
	"encoding/json"
	"errors"
	"image"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// 라이브 화면 (F-13): 페이지의 글자·위젯 위치(픽셀)를 받아 칸 격자로 옮김. 페이지에서 온 글자는 모두 brClean 을 거침.

const (
	brMaxSnapTexts = 8000
	brMaxSnapElems = 1500
	brCoordLimit   = 1e6
)

// brSnapJS 는 페이지 안에서 보이는 글자 줄과 누를 수 있는 요소를 모아 JSON 문자열로 돌려줌
const brSnapJS = `(()=>{
const vw=innerWidth,vh=innerHeight;
const SEL='a[href],button,input:not([type=hidden]),select,textarea,summary,[role=button],[role=link],[role=tab],[role=menuitem],[role=checkbox],[role=switch],[onclick],[tabindex]:not([tabindex="-1"])';
const SKIP={SCRIPT:1,STYLE:1,NOSCRIPT:1,TEMPLATE:1,HEAD:1,SVG:1,OPTION:1,OPTGROUP:1,CANVAS:1};
const texts=[],els=[],recs=[];
let words=0,D=document,ox=0,oy=0,frames=0;
const ranges=new Map();
const rangeOf=d=>{let r=ranges.get(d);if(!r){r=d.createRange();ranges.set(d,r)}return r};
const win=n=>n.ownerDocument.defaultView||window;
const deepAt=(x,y)=>{let t=D.elementFromPoint(x,y);while(t&&t.shadowRoot){const i=t.shadowRoot.elementFromPoint(x,y);if(!i||i===t)break;t=i}return t};
const anc=(a,b)=>{for(let n=b;n;n=n.parentNode||n.host){if(n===a)return true}return false};
const onTop=(el,x,y)=>{if(x+ox<0||y+oy<0||x+ox>=vw||y+oy>=vh)return false;const t=deepAt(x,y);return !!t&&(anc(el,t)||anc(t,el))};
const clean=s=>(s||'').replace(/\s+/g,' ').trim();
const clampX=x=>Math.min(Math.max(x+ox,0),vw-1)-ox;
const clampY=y=>Math.min(Math.max(y+oy,0),vh-1)-oy;
function pushLine(c,ctx,el){
  if(!c)return;
  const cx=(c.l+c.r)/2,cy=(c.t+c.b)/2;
  if(cy+oy<0||cy+oy>=vh||c.r+ox<=0||c.l+ox>=vw)return;
  if(!onTop(el,clampX(cx),cy))return;
  texts.push({t:c.s,x:c.l+ox,y:c.t+oy,w:c.r-c.l,h:c.b-c.t,f:ctx.f,b:ctx.b?1:0,hd:ctx.hd,k:0});
}
function lines(tn,ctx,el){
  const range=rangeOf(tn.ownerDocument);
  range.selectNodeContents(tn);
  const rb=range.getBoundingClientRect();
  if(rb.width===0&&rb.height===0)return;
  if(rb.bottom+oy<0||rb.top+oy>vh)return;
  const s=tn.data,re=/\S+/g;let m,cur=null;
  const add=(str,r,sp)=>{
    if(cur&&Math.abs(r.top-cur.t)<r.height*0.5&&r.left>=cur.r-2){cur.s+=(sp?' ':'')+str;cur.r=r.right;cur.b=Math.max(cur.b,r.bottom);cur.t=Math.min(cur.t,r.top)}
    else{pushLine(cur,ctx,el);cur={s:str,l:r.left,r:r.right,t:r.top,b:r.bottom}}
  };
  while((m=re.exec(s))&&words<7000){
    range.setStart(tn,m.index);range.setEnd(tn,m.index+m[0].length);
    const rs=range.getClientRects();
    if(!rs.length)continue;
    words++;
    if(rs.length>1){
      for(let i=0;i<m[0].length;i++){
        range.setStart(tn,m.index+i);range.setEnd(tn,m.index+i+1);
        const r=range.getClientRects()[0];
        if(!r||r.width===0)continue;
        add(m[0][i],r,i===0);
      }
      continue;
    }
    const r=rs[0];
    if(r.width===0&&r.height===0)continue;
    add(m[0],r,true);
  }
  pushLine(cur,ctx,el);
}
function labelOf(e){
  return clean(e.getAttribute('aria-label')||e.innerText||e.title||(e.querySelector&&e.querySelector('img[alt]')&&e.querySelector('img[alt]').alt)||'').slice(0,80);
}
function rec(e){
  const r=e.getBoundingClientRect();
  if(r.width<2||r.height<2||r.bottom+oy<=0||r.top+oy>=vh||r.right+ox<=0||r.left+ox>=vw)return null;
  if(!onTop(e,clampX(r.left+r.width/2),clampY(r.top+r.height/2)))return null;
  const tag=e.tagName,role=(e.getAttribute('role')||'').toLowerCase();
  let kind='link',type='',label='',value='',ph='',checked=0,widget=0;
  if(tag==='INPUT'){
    type=(e.type||'text').toLowerCase();
    if(type==='checkbox'||type==='radio'){kind='check';checked=e.checked?1:0;widget=1}
    else if(['submit','button','reset','image'].includes(type)){kind='button';label=clean(e.value||e.alt||e.title||'');widget=1}
    else if(['file','range','color'].includes(type)){kind='other';widget=1;label=type}
    else{kind='input';value=type==='password'?'*'.repeat(Math.min(e.value.length,40)):(e.value||'').slice(0,200);ph=clean(e.placeholder||e.getAttribute('aria-label')||'').slice(0,80);widget=1}
  }else if(tag==='TEXTAREA'){kind='input';type='textarea';value=(e.value||'').replace(/\s+/g,' ').slice(0,200);ph=clean(e.placeholder||e.getAttribute('aria-label')||'').slice(0,80);widget=1}
  else if(tag==='SELECT'){kind='select';value=clean(e.selectedOptions[0]?e.selectedOptions[0].text:'').slice(0,60);widget=1}
  else if(tag==='BUTTON'||role==='button'||role==='tab'||role==='menuitem'||role==='switch'){kind='button';label=labelOf(e);widget=(tag==='BUTTON'||r.height<60)&&label.length<40?1:0}
  else if(tag==='A'){kind='link';label=labelOf(e)}
  else{kind='action';label=labelOf(e)}
  if(!label&&(kind==='link'||kind==='button'||kind==='action'))label=labelOf(e);
  const hf=(tag==='A'&&e.href&&/^https?:/i.test(e.href))?e.href:'';
  return {k:kind,ty:type,l:label,v:value,p:ph,c:checked,wg:widget,hf:hf,x:r.left+ox,y:r.top+oy,w:r.width,h:r.height};
}
function visit(n,ctx,depth){
  if(depth>200)return;
  if(n.nodeType===3){
    if(n.data.trim())lines(n,ctx,n.parentElement||D.body);
    return;
  }
  if(n.nodeType!==1)return;
  const tag=n.tagName.toUpperCase();
  if(SKIP[tag]||n.hidden)return;
  const cs=win(n).getComputedStyle(n);
  if(cs.display==='none'||cs.visibility==='hidden'||+cs.opacity===0)return;
  if(tag==='IFRAME'||tag==='FRAME'){
    if(frames>=6)return;
    let d=null;try{d=n.contentDocument}catch(_){}
    const r=n.getBoundingClientRect();
    if(r.width<2||r.height<2||r.bottom+oy<0||r.top+oy>vh)return;
    if(!d){
      if(r.width>=48&&r.height>=24&&r.right+ox>0&&r.left+ox<vw){
        const top=Math.max(r.top+oy,0),bot=Math.min(r.bottom+oy,vh),ti=clean(n.title).slice(0,60);
        texts.push({t:ti?'[삽입: '+ti+']':'[삽입 콘텐츠]',x:r.left+ox,y:top,w:r.width,h:Math.min(bot-top,40),f:ctx.f,b:0,hd:0,k:1,th:bot-top});
      }
      return;
    }
    if(!d.body)return;
    frames++;
    const pD=D,pox=ox,poy=oy;
    D=d;ox=pox+r.left;oy=poy+r.top;
    visit(d.body,ctx,depth+1);
    D=pD;ox=pox;oy=poy;
    return;
  }
  let c=ctx;
  if(/^H[1-6]$/.test(tag))c={f:c.f,b:c.b,hd:+tag[1]};
  const w=parseInt(cs.fontWeight,10);
  if(w>=600&&!c.b)c={f:c.f,b:true,hd:c.hd};
  if(tag==='IMG'){
    const r=n.getBoundingClientRect();
    if(r.width>=48&&r.height>=24&&r.bottom+oy>0&&r.top+oy<vh){
      const alt=clean(n.alt).slice(0,60);
      const top=Math.max(r.top+oy,0),bot=Math.min(r.bottom+oy,vh);
      texts.push({t:alt?'[이미지: '+alt+']':'[이미지]',x:r.left+ox,y:top,w:r.width,h:Math.min(bot-top,40),f:c.f,b:0,hd:0,k:1,th:bot-top});
    }
    return;
  }
  if(tag==='VIDEO'){
    const r=n.getBoundingClientRect();
    if(r.width>=48&&r.bottom+oy>0&&r.top+oy<vh){const top=Math.max(r.top+oy,0),bot=Math.min(r.bottom+oy,vh);texts.push({t:'[영상]',x:r.left+ox,y:top,w:r.width,h:Math.min(bot-top,40),f:c.f,b:0,hd:0,k:1,th:bot-top})}
    return;
  }
  if(n.matches(SEL)){
    const skip=c.f>=0&&!(tag==='INPUT'||tag==='TEXTAREA'||tag==='SELECT'||tag==='BUTTON');
    if(!skip&&els.length<1500){
      const rc=rec(n);
      if(rc){els.push(n);recs.push(rc);c={f:els.length-1,b:c.b,hd:c.hd}}
    }
  }
  if(tag==='TEXTAREA'||tag==='SELECT'||tag==='INPUT')return;
  if(n.shadowRoot)for(let ch=n.shadowRoot.firstChild;ch;ch=ch.nextSibling)visit(ch,c,depth+1);
  for(let ch=n.firstChild;ch;ch=ch.nextSibling)visit(ch,c,depth+1);
}
visit(document.body||document.documentElement,{f:-1,b:false,hd:0},0);
window.__tui=els;
const se=document.scrollingElement||document.documentElement;
return JSON.stringify({u:location.href,t:document.title,sy:se.scrollTop,dh:se.scrollHeight,vh:vh,vw:vw,texts:texts,elems:recs});
})()`

// ---- 받은 내용 ----

type brSnapText struct {
	T  string  `json:"t"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	W  float64 `json:"w"`
	H  float64 `json:"h"`
	F  int     `json:"f"`
	B  int     `json:"b"`
	H1 int     `json:"hd"`
	K  int     `json:"k"`
	TH float64 `json:"th"`
}

type brSnapElem struct {
	Kind    string  `json:"k"`
	Type    string  `json:"ty"`
	Label   string  `json:"l"`
	Value   string  `json:"v"`
	Ph      string  `json:"p"`
	Checked int     `json:"c"`
	Href    string  `json:"hf"`
	Widget  int     `json:"wg"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	W       float64 `json:"w"`
	H       float64 `json:"h"`
}

type brSnap struct {
	URL   string       `json:"u"`
	Title string       `json:"t"`
	SY    float64      `json:"sy"`
	DH    float64      `json:"dh"`
	VH    float64      `json:"vh"`
	VW    float64      `json:"vw"`
	Texts []brSnapText `json:"texts"`
	Elems []brSnapElem `json:"elems"`
	Shot  image.Image `json:"-"`
}

func brFinite(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return math.Max(-brCoordLimit, math.Min(brCoordLimit, f))
}

// brParseSnap 은 페이지가 돌려준 JSON 을 읽어 글자를 거르고 좌표·개수를 제한함
func brParseSnap(raw string) (*brSnap, error) {
	if len(raw) > 16<<20 {
		return nil, errors.New("화면 정보가 너무 큼")
	}
	var s brSnap
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return nil, errors.New("화면 정보를 읽지 못함")
	}
	s.URL = brClip(brClean(s.URL), brMaxURL)
	s.Title = brClip(brCollapse(s.Title), 200)
	s.SY, s.DH, s.VH, s.VW = brFinite(s.SY), brFinite(s.DH), brFinite(s.VH), brFinite(s.VW)
	if len(s.Texts) > brMaxSnapTexts {
		s.Texts = s.Texts[:brMaxSnapTexts]
	}
	if len(s.Elems) > brMaxSnapElems {
		s.Elems = s.Elems[:brMaxSnapElems]
	}
	for i := range s.Texts {
		t := &s.Texts[i]
		t.T = brClip(brCollapse(t.T), 300)
		t.TH = math.Max(0, brFinite(t.TH))
		t.X, t.Y, t.W, t.H = brFinite(t.X), brFinite(t.Y), brFinite(t.W), math.Max(1, brFinite(t.H))
		if t.F >= len(s.Elems) || t.F < -1 {
			t.F = -1
		}
	}
	for i := range s.Elems {
		e := &s.Elems[i]
		e.Kind, e.Type = brClip(brClean(e.Kind), 12), brClip(brClean(e.Type), 12)
		e.Href = brClip(brClean(e.Href), brMaxURL)
		e.Label, e.Value, e.Ph = brClip(brCollapse(e.Label), 80), brClip(brCollapse(e.Value), 200), brClip(brCollapse(e.Ph), 80)
		e.X, e.Y, e.W, e.H = brFinite(e.X), brFinite(e.Y), brFinite(e.W), brFinite(e.H)
	}
	return &s, nil
}

// ---- 칸 격자 ----

type brItemKind uint8

const (
	biText brItemKind = iota
	biHead
	biLink
	biMuted
	biBracket // 버튼·입력창·체크박스 같은 위젯
	biHint    // 입력창의 안내 글(placeholder)
	biPic     // 이미지 (캡처에서 색을 따와 그림)
)

type brItem struct {
	col  int
	text string
	kind brItemKind
	bold bool
	elem int // 이 글자가 속한 누를 수 있는 요소 (없으면 -1)
	pic  *brPic
	band int // pic 안에서 몇 번째 칸 줄인지
}

func (it brItem) width() int { return lipgloss.Width(it.text) }

// brScreen 은 칸 격자로 옮긴 한 화면
type brScreen struct {
	rows   [][]brItem
	rowY   []float64 // 칸 줄마다 페이지 픽셀 높이 (클릭 위치)
	cols   int
	snap   *brSnap
	elemP  [][2]int // 요소마다 (줄, 칸) 위치. 보이지 않으면 (-1,-1)
	shiftX float64  // 왼쪽 빈 여백을 덜어낸 픽셀 (클릭 좌표를 되돌릴 때 씀)
}

type brCand struct {
	x, y, w, h float64
	tall       float64
	text       string
	kind       brItemKind
	bold       bool
	elem       int
	pic        *brPic
	ext        float64
}

type brCluster struct {
	cy, top, bottom, h float64
	tall               float64
	items              []brCand
}

func brWrapBracket(inner string) string { return "[" + inner + "]" }

// brWidgetText 는 요소를 칸 격자에서 어떤 모양으로 그릴지 정함
func brWidgetText(e brSnapElem, cols int) (string, brItemKind, bool) {
	switch e.Kind {
	case "check":
		on := "x"
		if e.Checked == 0 {
			on = " "
		}
		if e.Type == "radio" {
			if e.Checked == 0 {
				return "( )", biBracket, true
			}
			return "(*)", biBracket, true
		}
		return brWrapBracket(on), biBracket, true
	case "input":
		w := int(math.Round(e.W / brCellW))
		w = max(10, min(w, cols-2))
		inner := w - 4
		txt, kind := e.Value, biText
		if txt == "" {
			txt, kind = e.Ph, biHint
		}
		txt = brTrunc(txt, inner)
		return "[ " + txt + sp(inner-lipgloss.Width(txt)) + " ]", kind, true
	case "select":
		return "[ " + brTrunc(e.Value, 24) + " v ]", biBracket, true
	case "button":
		if e.Widget == 1 {
			l := e.Label
			if l == "" {
				l = "·"
			}
			return "[ " + brTrunc(l, 40) + " ]", biBracket, true
		}
	case "other":
		return "[ " + e.Label + " ]", biBracket, true
	}
	return "", biText, false
}

// brLayoutSnap 은 스냅샷을 cols 칸 폭의 줄들로 옮김
func brLayoutSnap(s *brSnap, cols int) *brScreen {
	sc := &brScreen{cols: cols, snap: s}
	hasText := make([]bool, len(s.Elems))
	var cands []brCand
	for _, t := range s.Texts {
		if t.T == "" {
			continue
		}
		if t.F >= len(s.Elems) || t.F < -1 {
			t.F = -1
		}
		if t.F >= 0 && s.Elems[t.F].Widget == 1 && s.Elems[t.F].Kind != "link" {
			continue
		}
		kind := biText
		switch {
		case t.H1 > 0:
			kind = biHead
		case t.K == 1:
			kind = biMuted
		case t.F >= 0 && (s.Elems[t.F].Kind == "link" || s.Elems[t.F].Kind == "action"):
			kind = biLink
		}
		if t.F >= 0 {
			hasText[t.F] = true
		}
		if t.K == 1 && s.Shot != nil {
			if c, ok := brPicCand(s, t, cols); ok {
				cands = append(cands, c)
				continue
			}
		}
		cands = append(cands, brCand{x: t.X, y: t.Y, w: t.W, h: t.H, text: t.T, kind: kind, bold: t.B == 1 || t.H1 > 0, elem: t.F, tall: t.TH})
	}
	for i, e := range s.Elems {
		txt, kind, ok := brWidgetText(e, cols)
		if !ok {
			if hasText[i] {
				continue
			}
			l := e.Label
			if l == "" {
				l = "링크"
			}
			txt, kind = brWrapBracket(brTrunc(l, 40)), biLink
		}
		top, bot := e.Y, e.Y+e.H
		if s.VH > 0 {
			top, bot = math.Max(top, 0), math.Min(bot, s.VH)
		}
		cands = append(cands, brCand{x: e.X, y: top, w: e.W, h: math.Max(1, math.Min(bot-top, 40)), text: txt, kind: kind, elem: i})
	}
	if len(cands) > 0 {
		minX := cands[0].x
		for _, c := range cands {
			minX = math.Min(minX, c.x)
		}
		if sc.shiftX = math.Max(0, minX-brCellW); sc.shiftX > 0 {
			for i := range cands {
				cands[i].x -= sc.shiftX
			}
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		ci, cj := cands[i].y+cands[i].h/2, cands[j].y+cands[j].h/2
		if ci != cj {
			return ci < cj
		}
		return cands[i].x < cands[j].x
	})
	var cls []*brCluster
	for _, c := range cands {
		cy := c.y + c.h/2
		if n := len(cls); n > 0 && cy-cls[n-1].cy <= math.Min(cls[n-1].h, c.h)*0.55 {
			k := cls[n-1]
			k.cy = (k.cy*float64(len(k.items)) + cy) / float64(len(k.items)+1)
			k.top, k.bottom = math.Min(k.top, c.y), math.Max(k.bottom, c.y+math.Max(c.h, c.ext))
			k.h = math.Max(k.h, c.h)
			k.tall = math.Max(k.tall, c.tall)
			k.items = append(k.items, c)
			continue
		}
		cls = append(cls, &brCluster{cy: cy, top: c.y, bottom: c.y + math.Max(c.h, c.ext), h: c.h, tall: c.tall, items: []brCand{c}})
	}
	sc.elemP = make([][2]int, len(s.Elems))
	for i := range sc.elemP {
		sc.elemP[i] = [2]int{-1, -1}
	}
	row := 0
	for ci, k := range cls {
		if ci > 0 {
			gap := k.top - cls[ci-1].bottom
			if gap > math.Max(10, k.h*0.5) {
				sc.rows = append(sc.rows, nil)
				sc.rowY = append(sc.rowY, (cls[ci-1].bottom+k.top)/2)
				row++
			}
		}
		main, extra := brPlaceRow(k.items, cols)
		sc.rows = append(sc.rows, main)
		sc.rowY = append(sc.rowY, k.cy)
		for _, it := range sc.rows[len(sc.rows)-1] {
			if it.elem >= 0 && sc.elemP[it.elem][0] < 0 {
				sc.elemP[it.elem] = [2]int{row, it.col}
			}
		}
		row++
		for _, ex := range extra {
			sc.rows = append(sc.rows, ex)
			sc.rowY = append(sc.rowY, k.cy)
			row++
		}
		row = brPicRowsBelow(sc, main, row)
		if k.tall >= 72 {
			n := min(7, int(k.tall/36)-1)
			for i := 1; i <= n; i++ {
				sc.rows = append(sc.rows, nil)
				sc.rowY = append(sc.rowY, k.top+float64(i)*k.tall/float64(n+1))
				row++
			}
		}
	}
	return sc
}

// brWordWrap 은 공백에서 줄을 바꾸고, 한 낱말이 w 보다 길면 글자 단위로 자름
func brWordWrap(s string, w int) []string {
	if w < 1 {
		return nil
	}
	var out []string
	cur := ""
	for _, word := range strings.Fields(s) {
		for lipgloss.Width(word) > w {
			if cur != "" {
				out, cur = append(out, cur), ""
			}
			parts := brHardWrap(word, w)
			if len(parts) < 2 { // 칸이 모자라 더 못 쪼개면 들어가는 만큼만 남김 (무한 반복 방지)
				word = brTrunc(word, w)
				break
			}
			out = append(out, parts[:len(parts)-1]...)
			word = parts[len(parts)-1]
		}
		if word == "" {
			continue
		}
		switch {
		case cur == "":
			cur = word
		case lipgloss.Width(cur)+1+lipgloss.Width(word) <= w:
			cur += " " + word
		default:
			out, cur = append(out, cur), word
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// brPlaceRow 는 한 줄의 항목을 칸에 놓음. 폭을 넘는 글자는 잘라 버리지 않고 이어지는 줄(extra)로 보냄.
func brPlaceRow(items []brCand, cols int) ([]brItem, [][]brItem) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].x < items[j].x })
	var out []brItem
	var extra [][]brItem
	prevEnd, prevElem, prevRight := 0, -2, -1e9
	for _, c := range items {
		col := int(math.Round(c.x / brCellW))
		if col < prevEnd {
			col = prevEnd
		}
		if col == prevEnd && len(out) > 0 && (c.elem != prevElem || c.x-prevRight >= 2) {
			col = prevEnd + 1
		}
		if w := lipgloss.Width(c.text); col+w > cols && cols-w >= prevEnd {
			col = max(prevEnd, cols-w)
		}
		if col >= cols {
			continue
		}
		txt := c.text
		if c.kind == biPic && lipgloss.Width(txt) > cols-col {
			txt = brSpaces(cols - col)
		}
		if c.kind == biBracket && lipgloss.Width(txt) > cols-col {
			txt = brTrunc(txt, cols-col)
		}
		if lipgloss.Width(txt) > cols-col {
			parts := brWordWrap(txt, cols-col)
			if len(parts) == 0 {
				continue
			}
			txt = parts[0]
			for n, p := range parts[1:] {
				for len(extra) <= n {
					extra = append(extra, nil)
				}
				extra[n] = append(extra[n], brItem{col: col, text: p, kind: c.kind, bold: c.bold, elem: c.elem})
			}
		}
		it := brItem{col: col, text: txt, kind: c.kind, bold: c.bold, elem: c.elem, pic: c.pic}
		out = append(out, it)
		prevEnd, prevElem, prevRight = col+it.width(), c.elem, c.x+c.w
	}
	return out, extra
}

// ---- 커서 ----

// cellPx 는 칸 위치(줄, 칸)에 해당하는 페이지 픽셀 좌표
func (sc *brScreen) cellPx(row, col int) (x, y float64) {
	x = float64(col)*brCellW + brCellW/2 + sc.shiftX
	switch {
	case len(sc.rowY) == 0:
		y = float64(row)*brCellH + brCellH/2
	case row < len(sc.rowY):
		y = sc.rowY[max(0, row)]
	default:
		y = sc.rowY[len(sc.rowY)-1] + float64(row-len(sc.rowY)+1)*brCellH
	}
	if sc.snap != nil && sc.snap.VH > 1 {
		y = math.Max(1, math.Min(y, sc.snap.VH-1))
	}
	return x, y
}

// elemAt 은 페이지 픽셀 좌표 아래에 있는 가장 작은 요소의 번호 (없으면 -1)
func (sc *brScreen) elemAt(x, y float64) int {
	best, area := -1, math.MaxFloat64
	for i, e := range sc.snap.Elems {
		if x >= e.X && x <= e.X+e.W && y >= e.Y && y <= e.Y+e.H && e.W*e.H < area {
			best, area = i, e.W*e.H
		}
	}
	return best
}

// nextAnchor 는 커서에서 다음(dir>0) 또는 이전 요소 위치로 (줄, 칸). 없으면 false.
func (sc *brScreen) nextAnchor(row, col, dir int) (int, int, bool) {
	var pos [][2]int
	for _, p := range sc.elemP {
		if p[0] >= 0 {
			pos = append(pos, p)
		}
	}
	sort.Slice(pos, func(i, j int) bool {
		if pos[i][0] != pos[j][0] {
			return pos[i][0] < pos[j][0]
		}
		return pos[i][1] < pos[j][1]
	})
	if len(pos) == 0 {
		return 0, 0, false
	}
	if dir > 0 {
		for _, p := range pos {
			if p[0] > row || (p[0] == row && p[1] > col) {
				return p[0], p[1], true
			}
		}
		return pos[0][0], pos[0][1], true
	}
	for i := len(pos) - 1; i >= 0; i-- {
		if pos[i][0] < row || (pos[i][0] == row && pos[i][1] < col) {
			return pos[i][0], pos[i][1], true
		}
	}
	last := pos[len(pos)-1]
	return last[0], last[1], true
}

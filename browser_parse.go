package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"golang.org/x/net/idna"
	xunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// 웹 브라우저(텍스트) F-13 의 내려받기 / 정리 / 줄바꿈 부분.
// 페이지에서 온 글자는 모두 믿을 수 없는 입력이므로 brClean 를 거쳐야 화면에 닿음.

const (
	brMaxBody      = 2 << 20 // 본문은 2MB 까지만 읽음
	brMaxRedirects = 4
	brMaxURL       = 2048
	brMaxLinks     = 999
	brMaxDepth     = 400    // DOM 을 이 깊이까지만 따라감
	brMaxChars     = 800000 // 문서 하나에서 정리해 담는 글자 수 상한
	brMaxBlocks    = 30000
	brMaxBlockLen  = 20000 // 문단 하나의 글자 수 상한
	brMaxPreLine   = 2000  // pre 한 줄의 글자 수 상한
	brUserAgent    = "TUI-playground-textbrowser/1.0 (terminal text reader; no JavaScript; not a crawler)"
)

// 테스트에서 줄이거나 바꿔 끼울 수 있도록 변수로 둠
var (
	browserTimeout = 10 * time.Second
	browserFetch   = brFetchPage
)

var (
	errBrCanceled     = errors.New("불러오기를 취소함")
	errBrTooManyRedir = errors.New("리다이렉트가 너무 많음 (최대 4번)")
)

// ---- 글자 정리 (보안) ----

// brClean 는 한 줄짜리 글에서 화면을 건드릴 수 있는 글자를 모두 없앰:
// ESC 를 포함한 C0/C1 제어 문자, 방향 제어·제로폭 같은 서식 문자(Cf), 사설 영역, 깨진 바이트.
// 줄바꿈·탭·특수 공백은 일반 공백 한 칸으로 바꿈.
func brClean(s string) string {
	ok := true
	for _, r := range s {
		if r < 0x20 || r >= 0x7f {
			ok = false
			break
		}
	}
	if ok {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		i += n
		if r == utf8.RuneError && n == 1 {
			continue
		}
		b.WriteString(brCleanRune(r))
	}
	return b.String()
}

func brCleanRune(r rune) string {
	switch {
	case r == ' ' || (r > 0x20 && r < 0x7f):
		return string(r)
	case r == 0x2028 || r == 0x2029 || unicode.IsSpace(r):
		return " "
	case unicode.IsControl(r), unicode.Is(unicode.Cf, r), unicode.Is(unicode.Co, r), unicode.Is(unicode.Cs, r):
		return ""
	case r == 0xfffe || r == 0xffff:
		return ""
	case r == 0xfe0e || r == 0xfe0f || r == 0x20e3 || (r >= 0x1f3fb && r <= 0x1f3ff):
		// 이모지 변형 선택자·피부색·키캡: 글자 하나씩 잰 폭과 터미널이 그리는 폭이 달라지므로 뺌
		return ""
	}
	return string(r)
}

// brTrunc 는 truncW 로 자른 뒤에도 lipgloss.Width 가 w 를 넘으면 한 글자씩 더 줄임
func brTrunc(s string, w int) string {
	s = truncW(s, w)
	for lipgloss.Width(s) > w && s != "" {
		rs := []rune(s)
		s = string(rs[:len(rs)-1])
	}
	return s
}

// brCleanPre 는 줄 구분은 살리고(\n) 탭은 공백 4칸으로 바꾸며, 나머지는 brClean 와 같이 거름. 한 줄은 brMaxPreLine 글자까지.
func brCleanPre(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		l = strings.ReplaceAll(l, "\t", "    ")
		l = brClean(l)
		lines[i] = brClip(l, brMaxPreLine)
	}
	return strings.Join(lines, "\n")
}

func brClip(s string, n int) string {
	if len(s) <= n { // 바이트 수가 이미 n 이하면 글자 수도 n 이하
		return s
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	rs := []rune(s)
	return string(rs[:n]) + "…"
}

// ---- 주소 ----

// brNormalizeURL 은 사용자가 친 주소를 http/https 전체 주소로 바꿈. 그 밖의 형식은 오류.
func brNormalizeURL(in string) (string, error) {
	s := strings.TrimSpace(in)
	if s == "" {
		return "", errors.New("주소가 비어 있음")
	}
	if len(s) > brMaxURL {
		return "", errors.New("주소가 너무 김")
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return "", errors.New("주소에 공백이나 제어 문자를 넣을 수 없음")
		}
	}
	switch scheme, rest, has := brSplitScheme(s); {
	case !has:
		if strings.HasPrefix(s, "//") {
			s = "https:" + s
		} else {
			s = "https://" + s
		}
	case strings.HasPrefix(rest, "//"):
		if l := strings.ToLower(scheme); l != "http" && l != "https" {
			return "", fmt.Errorf("http, https 주소만 열 수 있음 (입력한 형식: %s)", brClip(brClean(l), 20))
		}
	case brIsPortRest(rest): // localhost:8080 처럼 스킴이 아니라 호스트:포트
		s = "https://" + s
	default:
		return "", fmt.Errorf("http, https 주소만 열 수 있음 (입력한 형식: %s)", brClip(brClean(strings.ToLower(scheme)), 20))
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", errors.New("주소 형식이 올바르지 않음")
	}
	if sc := strings.ToLower(u.Scheme); sc != "http" && sc != "https" {
		return "", errors.New("http, https 주소만 열 수 있음")
	}
	if u.Hostname() == "" {
		return "", errors.New("주소에 호스트가 없음")
	}
	if u.User != nil {
		return "", errors.New("주소에 계정 정보를 넣을 수 없음")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Fragment = ""
	if !brFixHost(u) {
		return "", errors.New("호스트 이름이 올바르지 않음")
	}
	return u.String(), nil
}

// brFixHost 는 한글 도메인 같은 비ASCII 호스트를 punycode(xn--...)로 바꿈. 바꿀 수 없으면 false.
func brFixHost(u *url.URL) bool {
	h := u.Hostname()
	ascii := true
	for i := 0; i < len(h); i++ {
		if h[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return true
	}
	a, err := idna.Lookup.ToASCII(h)
	if err != nil || a == "" {
		return false
	}
	if p := u.Port(); p != "" {
		a = net.JoinHostPort(a, p)
	}
	u.Host = a
	return true
}

// brSplitScheme 는 `abc:` 꼴의 앞부분을 떼어냄
func brSplitScheme(s string) (scheme, rest string, ok bool) {
	for i, r := range s {
		switch {
		case r == ':':
			if i == 0 {
				return "", "", false
			}
			return s[:i], s[i+1:], true
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && (r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.'):
		default:
			return "", "", false
		}
	}
	return "", "", false
}

// brIsPortRest 는 `8080`, `8080/path` 처럼 포트 번호로 시작하는지
func brIsPortRest(rest string) bool {
	n := 0
	for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
		n++
	}
	return n > 0 && n <= 5 && (n == len(rest) || strings.ContainsRune("/?#", rune(rest[n])))
}

// brResolveHref 는 링크 주소를 기준 주소에 맞춰 절대 주소로 만듦. http/https 가 아니거나 같은 문서 안 이동(#)이면 false.
func brResolveHref(base *url.URL, href string) (string, bool) {
	href = strings.TrimSpace(href)
	href = strings.NewReplacer("\t", "", "\n", "", "\r", "").Replace(href)
	if href == "" || strings.HasPrefix(href, "#") || len(href) > brMaxURL {
		return "", false
	}
	ref, err := url.Parse(href)
	if err != nil {
		return "", false
	}
	if ref.Scheme != "" && !strings.EqualFold(ref.Scheme, "http") && !strings.EqualFold(ref.Scheme, "https") {
		return "", false
	}
	if base == nil {
		return "", false
	}
	abs := base.ResolveReference(ref)
	if (abs.Scheme != "http" && abs.Scheme != "https") || abs.Hostname() == "" || abs.User != nil {
		return "", false
	}
	abs.Fragment = ""
	if !brFixHost(abs) {
		return "", false
	}
	s := abs.String()
	if len(s) > brMaxURL {
		return "", false
	}
	return brClean(s), true
}

// brSameSite 는 두 주소의 호스트가 같은지 (www. 와 포트는 무시)
func brSameSite(a, b string) bool {
	ha, hb := brSiteHost(a), brSiteHost(b)
	return ha != "" && ha == hb
}

func brSiteHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// ---- 문서 구조 ----

type brStyle uint8

const (
	bsText  brStyle = iota
	bsHead          // 제목: 굵은 accent
	bsLink          // 링크 글자: accent
	bsNum           // 링크 번호 [n]: muted
	bsMuted         // 인용·코드·이미지 표시: muted
)

const bsBold brStyle = 0x80

func (s brStyle) kind() brStyle { return s &^ bsBold }

type brRun struct {
	t  string
	st brStyle
}

type brKind uint8

const (
	bkPara brKind = iota
	bkHead
	bkPre
	bkRule
)

type brBlock struct {
	kind        brKind
	runs        []brRun
	first, rest string // 첫 줄 앞과 이어지는 줄 앞에 붙는 들여쓰기(인용 막대, 목록 기호)
	tight       bool   // 목록 항목·표 줄: 앞 블록도 tight 이면 빈 줄을 넣지 않음
}

type brPage struct {
	URL       string
	Title     string
	Blocks    []brBlock
	Links     []string
	Truncated bool // 본문이 2MB 를 넘어 잘림
	Cut       bool // 문서가 너무 커서 일부만 정리함
}

// ---- 내려받기 ----

func brFetchPage(ctx context.Context, raw string) (*brPage, error) {
	norm, err := brNormalizeURL(raw)
	if err != nil {
		return nil, err
	}
	norm = brSiteRewrite(norm)
	ctx, cancel := context.WithTimeout(ctx, browserTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, norm, nil)
	if err != nil {
		return nil, errors.New("주소 형식이 올바르지 않음")
	}
	req.Header.Set("User-Agent", brUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.8,*/*;q=0.1")
	req.Header.Set("Accept-Language", "ko,en;q=0.8")

	tr := &http.Transport{
		Proxy:                  http.ProxyFromEnvironment,
		DialContext:            (&net.Dialer{Timeout: browserTimeout}).DialContext,
		TLSHandshakeTimeout:    browserTimeout,
		DisableKeepAlives:      true, // 끝난 연결이 남아 고루틴을 붙들지 않게 함
		MaxResponseHeaderBytes: 64 << 10,
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{ // 쿠키 저장소(Jar) 없음, 인증 정보 없음
		Transport: tr,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) > brMaxRedirects {
				return errBrTooManyRedir
			}
			if r.URL.User != nil {
				return errors.New("계정 정보가 든 주소로 이동하려 해서 막음")
			}
			r.Header.Del("Referer")
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, brClassifyErr(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d %s (이동할 주소가 없음)", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	ctype := resp.Header.Get("Content-Type")
	mt, _, _ := mime.ParseMediaType(ctype)
	if mt != "" && !brRenderable(mt) {
		return nil, fmt.Errorf("HTML 이 아닌 형식이라 표시하지 않음: %s", brClip(brClean(mt), 60))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, brMaxBody+1))
	if err != nil {
		return nil, brClassifyErr(ctx, err)
	}
	truncated := len(body) > brMaxBody
	if truncated {
		body = body[:brMaxBody]
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("내용이 없음")
	}
	if mt == "" { // 헤더에 형식이 없으면 앞부분을 보고 판단함
		sniff := body
		if len(sniff) > 512 {
			sniff = sniff[:512]
		}
		mt, _, _ = mime.ParseMediaType(http.DetectContentType(sniff))
		if !brRenderable(mt) {
			return nil, fmt.Errorf("HTML 이 아닌 형식이라 표시하지 않음: %s", brClip(brClean(mt), 60))
		}
	}
	final := resp.Request.URL
	p := brBuildPage(body, ctype, mt == "text/plain", final)
	p.Truncated = truncated
	return p, nil
}

func brRenderable(mt string) bool {
	return mt == "text/html" || mt == "application/xhtml+xml" || mt == "text/plain"
}

func brClassifyErr(ctx context.Context, err error) error {
	var dns *net.DNSError
	var ne net.Error
	var hostErr x509.HostnameError
	var authErr x509.UnknownAuthorityError
	var certErr x509.CertificateInvalidError
	switch {
	case errors.Is(err, errBrTooManyRedir):
		return errBrTooManyRedir
	case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("시간 초과 (%d초 안에 응답이 끝나지 않음)", int(browserTimeout.Seconds()))
	case errors.Is(ctx.Err(), context.Canceled), errors.Is(err, context.Canceled):
		return errBrCanceled
	case errors.As(err, &dns):
		return fmt.Errorf("주소를 찾을 수 없음: %s", brClip(brClean(dns.Name), 80))
	case errors.As(err, &hostErr), errors.As(err, &authErr), errors.As(err, &certErr):
		return errors.New("인증서를 믿을 수 없어 연결하지 않음")
	case errors.As(err, &ne) && ne.Timeout():
		return fmt.Errorf("시간 초과 (%d초 안에 응답이 끝나지 않음)", int(browserTimeout.Seconds()))
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return fmt.Errorf("연결 실패: %s", brClip(brClean(err.Error()), 100))
}

// ---- 글자 코드 변환 ----

// brDecodeBody 는 Content-Type 의 charset, BOM, <meta charset> 순으로 문자 코드를 정해 UTF-8 로 바꿈.
func brDecodeBody(body []byte, ctype string) io.Reader {
	ctype = brCharsetAlias(ctype)
	enc, name, certain := charset.DetermineEncoding(body, ctype)
	if !certain && name == "windows-1252" && brUTF8Tolerant(body) { // 아무 표시가 없는데 UTF-8 로 읽히면 UTF-8
		enc = xunicode.UTF8
	}
	return transform.NewReader(bytes.NewReader(body), enc.NewDecoder())
}

// brCharsetAlias 는 웹 표준 이름표에 없는 한국어 charset 이름(cp949 등)을 euc-kr 로 바꿔 줌 (euc-kr 디코더가 CP949 를 포함함)
func brCharsetAlias(ctype string) string {
	mt, params, err := mime.ParseMediaType(ctype)
	if err != nil {
		return ctype
	}
	switch strings.ToLower(params["charset"]) {
	case "cp949", "ms949", "uhc", "x-windows-949", "windows-949", "ksc5601", "ks_c_5601":
		return mt + "; charset=euc-kr"
	}
	return ctype
}

// brUTF8Tolerant: 끝이 잘려 글자 일부만 남은 경우를 빼고 UTF-8 로 읽히는지
func brUTF8Tolerant(b []byte) bool {
	for k := 0; k < 4 && k <= len(b); k++ {
		if utf8.Valid(b[:len(b)-k]) {
			return true
		}
	}
	return false
}

// brBuildPage 는 받은 바이트를 읽을 수 있는 문서로 정리함
func brBuildPage(body []byte, ctype string, plainText bool, final *url.URL) *brPage {
	u := *final
	u.User, u.Fragment = nil, ""
	p := &brPage{URL: brClean(u.String())}
	if plainText {
		raw, _ := io.ReadAll(brDecodeBody(body, ctype))
		p.Title = "텍스트 문서"
		txt := brCleanPre(string(raw))
		if len(txt) > brMaxChars {
			txt, p.Cut = brClip(txt, brMaxChars), true
		}
		p.Blocks = []brBlock{{kind: bkPre, runs: []brRun{{t: txt, st: bsMuted}}, first: "  ", rest: "  "}}
		return p
	}
	doc, err := html.Parse(brDecodeBody(body, ctype))
	if err != nil || doc == nil {
		p.Title = "읽을 수 없는 문서"
		p.Blocks = []brBlock{{kind: bkPara, runs: []brRun{{t: "문서 구조가 너무 복잡하거나 깨져 있어 읽을 수 없음", st: bsMuted}}}}
		return p
	}
	brExtract(p, doc, &u)
	return p
}

// ---- 본문 추출 (readability-lite) ----

type brScan struct {
	title, h1, body *html.Node
	articles, mains []*html.Node
	roleMains       []*html.Node
	named           []*html.Node // 알려진 글 본문 영역 (네이버 블로그 등). article/main 보다 먼저 고름
	baseHref        string
	scanned         int
}

func brScanDoc(n *html.Node, s *brScan, depth int) {
	if depth > brMaxDepth || s.scanned > 400000 {
		return
	}
	s.scanned++
	if n.Type == html.ElementNode {
		switch n.Data {
		case "title":
			if s.title == nil {
				s.title = n
			}
		case "h1":
			if s.h1 == nil {
				s.h1 = n
			}
		case "body":
			if s.body == nil {
				s.body = n
			}
		case "article":
			s.articles = append(s.articles, n)
		case "main":
			s.mains = append(s.mains, n)
		case "base":
			if s.baseHref == "" {
				s.baseHref = brAttr(n, "href")
			}
		}
		if brIsKnownBody(n) {
			s.named = append(s.named, n)
		}
		if strings.EqualFold(brAttr(n, "role"), "main") && n.Data != "main" {
			s.roleMains = append(s.roleMains, n)
		}
		switch n.Data {
		case "script", "style", "svg", "template", "noscript":
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		brScanDoc(c, s, depth+1)
	}
}

func brAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func brHasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

// brTextLen 은 눈에 보이는 글자 수를 대충 셈 (본문 후보를 고르는 용도)
func brTextLen(n *html.Node, depth int) int {
	if depth > brMaxDepth {
		return 0
	}
	if n.Type == html.TextNode {
		return len(strings.TrimSpace(n.Data))
	}
	if n.Type == html.ElementNode {
		switch n.Data {
		case "script", "style", "svg", "template", "noscript", "nav", "aside", "footer":
			return 0
		}
	}
	t := 0
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		t += brTextLen(c, depth+1)
	}
	return t
}

func brCollapse(s string) string { return strings.Join(strings.Fields(brClean(s)), " ") }

func brNodeText(n *html.Node, depth int, b *strings.Builder) {
	if depth > brMaxDepth || b.Len() > 4000 {
		return
	}
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
		return
	}
	if n.Type == html.ElementNode {
		switch n.Data {
		case "script", "style", "svg", "template", "noscript":
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		brNodeText(c, depth+1, b)
	}
}

func brExtract(p *brPage, doc *html.Node, final *url.URL) {
	var s brScan
	brScanDoc(doc, &s, 0)

	base := final
	if s.baseHref != "" {
		if r, ok := brResolveHref(final, s.baseHref); ok {
			if bu, err := url.Parse(r); err == nil {
				base = bu
			}
		}
	}
	if s.title != nil {
		var b strings.Builder
		brNodeText(s.title, 0, &b)
		p.Title = brClip(brCollapse(b.String()), 200)
	}
	if p.Title == "" && s.h1 != nil {
		var b strings.Builder
		brNodeText(s.h1, 0, &b)
		p.Title = brClip(brCollapse(b.String()), 200)
	}

	body := s.body
	if body == nil {
		body = doc
	}
	// 본문 후보: article 이 하나면 그것, 아니면 main, role=main 순. 글이 너무 짧으면 body 로 물러남.
	var cands []*html.Node
	cands = append(cands, s.named...)
	if len(s.articles) == 1 {
		cands = append(cands, s.articles[0])
	}
	cands = append(cands, s.mains...)
	cands = append(cands, s.roleMains...)
	root := body
	for _, c := range cands {
		if brTextLen(c, 0) >= 140 {
			root = c
			break
		}
	}

	try := func(root *html.Node, chrome, heur bool) *brWalker {
		w := brNewWalker(base, chrome, heur)
		if root != body && root.Type == html.ElementNode {
			w.inContent = 1
		}
		w.children(root, bsText, 0)
		w.flush(bkPara)
		return w
	}
	w := try(root, true, root == body)
	if w.vis < 80 && root != body {
		w = try(body, true, true)
	}
	if w.vis < 80 {
		if w2 := try(body, false, false); w2.vis > w.vis {
			w = w2
		}
	}
	p.Blocks, p.Links, p.Cut = w.blocks, w.links, w.cut
	if p.Title == "" {
		p.Title = brSiteHost(p.URL)
	}
}

// ---- DOM 걷기 ----

var brSkipTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "svg": true, "iframe": true, "object": true, "embed": true,
	"template": true, "head": true, "title": true, "canvas": true, "audio": true, "video": true, "map": true,
	"input": true, "button": true, "select": true, "textarea": true, "option": true, "optgroup": true,
	"datalist": true, "label": true, "dialog": true, "frame": true, "frameset": true, "math": true,
}

var brBlockTags = map[string]bool{
	"p": true, "div": true, "section": true, "article": true, "main": true, "header": true, "footer": true,
	"aside": true, "nav": true, "figure": true, "figcaption": true, "details": true, "summary": true,
	"address": true, "dl": true, "dd": true, "table": true, "thead": true, "tbody": true, "tfoot": true,
	"caption": true, "fieldset": true, "legend": true, "center": true, "hgroup": true, "body": true,
	"html": true, "form": true, "menu": true, "dir": true,
}

// 사이트 껍데기로 보는 class/id 낱말 (낱말 전체가 같을 때만)
var brChromeWords = map[string]bool{
	"nav": true, "navbar": true, "navigation": true, "menu": true, "sidebar": true, "footer": true,
	"cookie": true, "cookies": true, "advert": true, "ads": true, "popup": true, "modal": true,
	"breadcrumb": true, "breadcrumbs": true, "comments": true, "comment-list": true, "share": true,
}

type brCtx struct {
	quote   bool
	ordered bool
	n       int
	marker  string
	used    bool
}

type brWalker struct {
	base      *url.URL
	chrome    bool // nav, aside, footer, (본문 밖) header 를 버림
	heur      bool // class/id 낱말로 껍데기를 버림
	runs      []brRun
	blocks    []brBlock
	stack     []brCtx
	links     []string
	linkIdx   map[string]int
	vis       int // 보이는 글자 수
	chars     int
	cut       bool
	inContent int
	firstCell bool
}

func brNewWalker(base *url.URL, chrome, heur bool) *brWalker {
	return &brWalker{base: base, chrome: chrome, heur: heur, linkIdx: map[string]int{}}
}

func (w *brWalker) children(n *html.Node, st brStyle, depth int) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.walk(c, st, depth+1)
	}
}

func brIsHidden(n *html.Node) bool {
	if brHasAttr(n, "hidden") {
		return true
	}
	if st := brAttr(n, "style"); st != "" {
		st = strings.ToLower(strings.ReplaceAll(st, " ", ""))
		return strings.Contains(st, "display:none") || strings.Contains(st, "visibility:hidden")
	}
	return false
}

func brLooksChrome(n *html.Node) bool {
	for _, key := range []string{"class", "id"} {
		for _, f := range strings.Fields(strings.ToLower(brAttr(n, key))) {
			if brChromeWords[f] {
				return true
			}
		}
	}
	return false
}

func (w *brWalker) walk(n *html.Node, st brStyle, depth int) {
	if depth > brMaxDepth || w.cut {
		return
	}
	switch n.Type {
	case html.TextNode:
		w.addText(n.Data, st)
		return
	case html.DocumentNode:
		w.children(n, st, depth)
		return
	case html.ElementNode:
	default:
		return
	}
	tag := n.Data
	if brSkipTags[tag] || brIsHidden(n) {
		return
	}
	if w.chrome {
		switch tag {
		case "nav", "aside", "footer":
			return
		case "header":
			if w.inContent == 0 {
				return
			}
		}
	}
	if w.heur && tag != "body" && tag != "html" && brLooksChrome(n) {
		return
	}
	switch tag {
	case "br":
		w.runs = append(w.runs, brRun{t: "\n", st: st})
	case "hr":
		w.flush(bkPara)
		w.addBlock(brBlock{kind: bkRule})
	case "img":
		alt := brClip(brCollapse(brAttr(n, "alt")), 80)
		txt := "[이미지]"
		if alt != "" {
			txt = "[이미지: " + alt + "]"
		}
		if st.kind() == bsText {
			w.addText(txt, bsMuted|(st&bsBold))
		} else {
			w.addText(txt, st)
		}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		w.flush(bkPara)
		w.children(n, bsHead, depth)
		w.flushHead(int(tag[1] - '0'))
	case "ul", "ol":
		w.flush(bkPara)
		c := brCtx{ordered: tag == "ol"}
		if v, err := strconv.Atoi(strings.TrimSpace(brAttr(n, "start"))); err == nil && tag == "ol" && v > 0 && v < 100000 {
			c.n = v - 1
		}
		w.stack = append(w.stack, c)
		w.children(n, st, depth)
		w.flush(bkPara)
		w.stack = w.stack[:len(w.stack)-1]
	case "li":
		w.flush(bkPara)
		if i := w.lastList(); i >= 0 {
			e := &w.stack[i]
			e.n++
			depthL := 0
			for _, x := range w.stack[:i] {
				if !x.quote {
					depthL++
				}
			}
			switch {
			case e.ordered:
				e.marker = strconv.Itoa(e.n) + ". "
			case depthL%2 == 0:
				e.marker = "• "
			default:
				e.marker = "◦ "
			}
			e.used = false
		}
		w.children(n, st, depth)
		w.flush(bkPara)
		if i := w.lastList(); i >= 0 {
			w.stack[i].marker, w.stack[i].used = "", true
		}
	case "blockquote":
		w.flush(bkPara)
		w.stack = append(w.stack, brCtx{quote: true})
		w.children(n, st, depth)
		w.flush(bkPara)
		w.stack = w.stack[:len(w.stack)-1]
	case "pre":
		w.flush(bkPara)
		var b strings.Builder
		brPreText(n, 0, &b)
		raw := b.String()
		if len(raw) > brMaxBlockLen*4 {
			raw, w.cut = raw[:brMaxBlockLen*4], true
		}
		txt := strings.Trim(brCleanPre(raw), "\n")
		if strings.TrimSpace(txt) != "" {
			_, rest, _ := w.prefix(false)
			w.vis += len(strings.TrimSpace(txt))
			w.chars += len(txt)
			w.addBlock(brBlock{kind: bkPre, runs: []brRun{{t: txt, st: bsMuted}}, first: rest + "  ", rest: rest + "  "})
		}
	case "a":
		href, ok := brResolveHref(w.base, brAttr(n, "href"))
		if !ok {
			w.children(n, st, depth)
			return
		}
		before := w.vis
		w.children(n, bsLink|(st&bsBold), depth)
		if w.vis > before {
			if num := w.linkNum(href); num > 0 {
				w.addText("["+strconv.Itoa(num)+"]", bsNum)
			}
		}
	case "code", "kbd", "samp", "tt":
		if st.kind() == bsText {
			st = bsMuted | (st & bsBold)
		}
		w.children(n, st, depth)
	case "b", "strong", "dt":
		w.children(n, st|bsBold, depth)
		if tag == "dt" {
			w.flush(bkPara)
		}
	case "tr":
		w.flush(bkPara)
		w.firstCell = true
		w.children(n, st, depth)
		w.flushTight()
	case "td", "th":
		if !w.firstCell && len(w.runs) > 0 {
			w.addText(" │ ", bsMuted)
		}
		w.firstCell = false
		w.children(n, st, depth)
	default:
		if brBlockTags[tag] {
			article := tag == "article" || tag == "main" || tag == "section"
			w.flush(bkPara)
			if article {
				w.inContent++
			}
			w.children(n, st, depth)
			w.flush(bkPara)
			if article {
				w.inContent--
			}
			return
		}
		w.children(n, st, depth)
	}
}

func brPreText(n *html.Node, depth int, b *strings.Builder) {
	if depth > brMaxDepth || b.Len() > brMaxBlockLen*4+1 {
		return
	}
	switch n.Type {
	case html.TextNode:
		b.WriteString(n.Data)
		return
	case html.ElementNode:
		if brSkipTags[n.Data] {
			return
		}
		if n.Data == "br" {
			b.WriteByte('\n')
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		brPreText(c, depth+1, b)
	}
}

func (w *brWalker) lastList() int {
	for i := len(w.stack) - 1; i >= 0; i-- {
		if !w.stack[i].quote {
			return i
		}
	}
	return -1
}

func (w *brWalker) linkNum(href string) int {
	if i, ok := w.linkIdx[href]; ok {
		return i
	}
	if len(w.links) >= brMaxLinks {
		return 0
	}
	w.links = append(w.links, href)
	w.linkIdx[href] = len(w.links)
	return len(w.links)
}

func (w *brWalker) addText(s string, st brStyle) {
	if w.cut {
		return
	}
	s = brClean(s)
	if s == "" {
		return
	}
	if len(s) > brMaxBlockLen {
		s = brClip(s, brMaxBlockLen)
	}
	for _, r := range s {
		if r != ' ' {
			w.vis++
		}
	}
	w.chars += len(s)
	if w.chars > brMaxChars {
		w.cut = true
		return
	}
	if n := len(w.runs); n > 0 && w.runs[n-1].st == st && w.runs[n-1].t != "\n" {
		w.runs[n-1].t += s
		return
	}
	w.runs = append(w.runs, brRun{t: s, st: st})
}

// prefix 는 지금 겹쳐 있는 인용·목록에 따른 들여쓰기를 돌려줌. consume 이면 목록 기호를 쓴 것으로 표시함.
func (w *brWalker) prefix(consume bool) (first, rest string, item bool) {
	var f, r strings.Builder
	for i := range w.stack {
		if i >= 8 { // 너무 깊은 들여쓰기는 무시
			break
		}
		e := &w.stack[i]
		if e.quote {
			f.WriteString("│ ")
			r.WriteString("│ ")
			continue
		}
		m := e.marker
		if m == "" {
			m = "  "
		}
		mw := lipgloss.Width(m)
		if e.marker != "" && !e.used {
			f.WriteString(m)
			item = true
		} else {
			f.WriteString(sp(mw))
		}
		r.WriteString(sp(mw))
		if consume {
			e.used = true
		}
	}
	return f.String(), r.String(), item
}

func (w *brWalker) addBlock(b brBlock) {
	if len(w.blocks) >= brMaxBlocks {
		w.cut = true
		return
	}
	w.blocks = append(w.blocks, b)
}

func (w *brWalker) takeRuns() []brRun {
	runs := w.runs
	w.runs = nil
	// 앞뒤 공백·줄바꿈 정리
	for len(runs) > 0 {
		r := &runs[0]
		r.t = strings.TrimLeft(r.t, " \n")
		if r.t != "" {
			break
		}
		runs = runs[1:]
	}
	for len(runs) > 0 {
		r := &runs[len(runs)-1]
		r.t = strings.TrimRight(r.t, " \n")
		if r.t != "" {
			break
		}
		runs = runs[:len(runs)-1]
	}
	return runs
}

func (w *brWalker) inQuote() bool {
	for _, e := range w.stack {
		if e.quote {
			return true
		}
	}
	return false
}

func (w *brWalker) flush(kind brKind) { w.flushAs(kind, false) }
func (w *brWalker) flushTight()       { w.flushAs(bkPara, true) }

func (w *brWalker) flushHead(level int) {
	runs := w.takeRuns()
	if len(runs) == 0 {
		return
	}
	first, rest, _ := w.prefix(true)
	runs = append([]brRun{{t: strings.Repeat("#", level) + " ", st: bsHead}}, runs...)
	w.addBlock(brBlock{kind: bkHead, runs: runs, first: first, rest: rest})
}

func (w *brWalker) flushAs(kind brKind, tight bool) {
	runs := w.takeRuns()
	if len(runs) == 0 {
		return
	}
	if w.inQuote() {
		for i := range runs {
			if runs[i].st.kind() == bsText {
				runs[i].st = bsMuted | (runs[i].st & bsBold)
			}
		}
	}
	first, rest, item := w.prefix(true)
	w.addBlock(brBlock{kind: kind, runs: runs, first: first, rest: rest, tight: tight || item})
}

// ---- 줄바꿈 ----

type brAtom struct {
	t  string
	st brStyle
}

type brUnit struct {
	atoms []brAtom
	w     int
	space bool // 앞에 공백이 있었음
	nl    bool // 강제 줄바꿈
}

// brIsWide 은 한 글자가 2칸을 차지하는 글자(한글·한자·가나·전각)인지. 이런 글자는 글자마다 줄을 나눌 수 있음.
func brIsWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115f,
		r >= 0x2e80 && r <= 0xa4cf && r != 0x303f,
		r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff,
		r >= 0xfe30 && r <= 0xfe4f,
		r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6,
		r >= 0x20000 && r <= 0x3fffd:
		return true
	}
	return false
}

func brTokenize(runs []brRun) []brUnit {
	var units []brUnit
	var cur brUnit
	open, pending := false, false
	finish := func() {
		if !open {
			return
		}
		for _, a := range cur.atoms {
			cur.w += lipgloss.Width(a.t)
		}
		units = append(units, cur)
		cur, open = brUnit{}, false
	}
	add := func(r rune, st brStyle) {
		if !open {
			cur, open = brUnit{space: pending}, true
			pending = false
		}
		if n := len(cur.atoms); n > 0 && cur.atoms[n-1].st == st {
			cur.atoms[n-1].t += string(r)
		} else {
			cur.atoms = append(cur.atoms, brAtom{t: string(r), st: st})
		}
	}
	for _, run := range runs {
		for _, r := range run.t {
			switch {
			case r == '\n':
				finish()
				units = append(units, brUnit{nl: true})
				pending = false
			case r == ' ':
				finish()
				pending = true
			case brIsWide(r):
				finish()
				add(r, run.st)
				finish()
			default:
				add(r, run.st)
			}
		}
	}
	finish()
	return units
}

type brLineB struct {
	atoms []brAtom
	w     int
}

func (l *brLineB) add(t string, st brStyle, w int) {
	if n := len(l.atoms); n > 0 && l.atoms[n-1].st == st {
		l.atoms[n-1].t += t
	} else {
		l.atoms = append(l.atoms, brAtom{t: t, st: st})
	}
	l.w += w
}

// brWrapRuns 는 글자 조각들을 폭 avail 칸에 맞춰 줄로 나눔. 영문은 공백에서, 한글·한자는 글자 사이에서 나누고
// 한 낱말이 한 줄보다 길면 강제로 자름. 어떤 줄도 avail 을 넘지 않음.
func brWrapRuns(runs []brRun, avail int) [][]brAtom {
	if avail < 1 {
		avail = 1
	}
	var out [][]brAtom
	var cur brLineB
	endLine := func() {
		out = append(out, cur.atoms)
		cur = brLineB{}
	}
	for _, u := range brTokenize(runs) {
		if u.nl {
			endLine()
			continue
		}
		need := u.w
		if u.space && cur.w > 0 {
			need++
		}
		if cur.w+need <= avail {
			if u.space && cur.w > 0 {
				cur.add(" ", bsText, 1)
			}
			for _, a := range u.atoms {
				cur.add(a.t, a.st, lipgloss.Width(a.t))
			}
			continue
		}
		if cur.w > 0 {
			endLine()
		}
		if u.w <= avail {
			for _, a := range u.atoms {
				cur.add(a.t, a.st, lipgloss.Width(a.t))
			}
			continue
		}
		// 한 낱말이 한 줄보다 김: 글자 단위로 강제로 자름
		for _, a := range u.atoms {
			for _, r := range a.t {
				rw := lipgloss.Width(string(r))
				if cur.w+rw > avail && cur.w > 0 {
					endLine()
				}
				cur.add(string(r), a.st, rw)
			}
		}
	}
	if cur.w > 0 || len(cur.atoms) > 0 {
		endLine()
	}
	return out
}

// ---- 화면용 줄 만들기 ----

type brLine []seg

func brLineText(l brLine) string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.t)
	}
	return b.String()
}

func brAtomSeg(a brAtom) seg {
	t := brClean(a.t) // 화면에 닿기 직전에 한 번 더 거름
	bold := a.st&bsBold != 0
	switch a.st.kind() {
	case bsHead:
		return sgBold(t, colorAccent)
	case bsLink:
		s := sg(t, colorAccent)
		s.bold = bold
		return s
	case bsNum, bsMuted:
		return sg(t, colorTextMuted)
	}
	s := sg(t, colorText)
	s.bold = bold
	return s
}

func brPrefixOK(first string, width int) bool { return lipgloss.Width(first) <= width-8 }

// brLayoutPage 는 문서를 폭 width 칸의 줄들로 펼침. 모든 줄의 표시 폭은 width 이하임.
func brLayoutPage(p *brPage, width int) []brLine {
	var lines []brLine
	blank := func() {
		if len(lines) > 0 && len(lines[len(lines)-1]) > 0 {
			lines = append(lines, nil)
		}
	}
	blocks := p.Blocks
	if len(p.Links) > 0 {
		blocks = append(append([]brBlock{}, blocks...), brBlock{kind: bkHead, runs: []brRun{{t: "링크 목록", st: bsHead}}})
		for i, l := range p.Links {
			blocks = append(blocks, brBlock{kind: bkPara, tight: true, first: "", rest: sp(len(strconv.Itoa(i+1)) + 3),
				runs: []brRun{{t: "[" + strconv.Itoa(i+1) + "] ", st: bsNum}, {t: l, st: bsMuted}}})
		}
	}
	prevTight := false
	for _, b := range blocks {
		first, rest := b.first, b.rest
		if !brPrefixOK(first, width) || !brPrefixOK(rest, width) {
			first, rest = "", ""
		}
		if !(b.tight && prevTight) {
			blank()
		}
		prevTight = b.tight
		pw := lipgloss.Width(first)
		switch b.kind {
		case bkRule:
			lines = append(lines, brLine{sg(strings.Repeat("─", min(width, 40)), colorTextFaint)})
		case bkPre:
			for _, raw := range strings.Split(b.runs[0].t, "\n") {
				if raw == "" {
					lines = append(lines, nil)
					continue
				}
				for _, ch := range brHardWrap(raw, width-pw) {
					lines = append(lines, brLine{sg(first+ch, colorTextMuted)})
				}
			}
		default:
			for i, atoms := range brWrapRuns(b.runs, width-pw) {
				pre := first
				if i > 0 {
					pre = rest
				}
				l := brLine{}
				if pre != "" {
					l = append(l, sg(pre, colorTextMuted))
				}
				for _, a := range atoms {
					l = append(l, brAtomSeg(a))
				}
				lines = append(lines, l)
			}
		}
	}
	for len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	// 마지막 방어: 어떤 줄도 폭을 넘지 않게 함
	for i, l := range lines {
		lines[i] = brFitLine(l, width)
	}
	return lines
}

// brHardWrap 은 공백을 살린 채 폭 w 칸 단위로 자름 (pre 용)
func brHardWrap(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	var out []string
	var b strings.Builder
	cw := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if cw+rw > w && cw > 0 {
			out = append(out, b.String())
			b.Reset()
			cw = 0
		}
		b.WriteRune(r)
		cw += rw
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// brFitLine 은 줄의 표시 폭이 w 를 넘으면 뒤에서부터 잘라냄
func brFitLine(l brLine, w int) brLine {
	if segsWidth(l) <= w {
		return l
	}
	var out brLine
	used := 0
	for _, s := range l {
		if used+s.width() <= w {
			out = append(out, s)
			used += s.width()
			continue
		}
		var b strings.Builder
		for _, r := range s.t {
			rw := lipgloss.Width(string(r))
			if used+rw > w {
				break
			}
			b.WriteRune(r)
			used += rw
		}
		s.t = b.String()
		out = append(out, s)
		break
	}
	// 글자 하나씩 더한 폭과 실제 폭이 다른 경우를 위한 마지막 방어
	for len(out) > 0 && segsWidth(out) > w {
		last := &out[len(out)-1]
		rs := []rune(last.t)
		if len(rs) == 0 {
			out = out[:len(out)-1]
			continue
		}
		last.t = string(rs[:len(rs)-1])
	}
	return out
}

// brSiteRewrite 는 자바스크립트가 본문을 따로 불러오는 사이트의 글 주소를, 서버가 완성된 HTML 로 주는 본문 주소로 바꿈.
// 지금은 네이버 블로그만 다룸: blog.naver.com/<아이디>/<글번호> 는 빈 틀(frameset)만 오고 본문은 스크립트가 만든 iframe 에 있음.
// 그 iframe 이 여는 PostView.naver 는 서버에서 본문을 완성해 주므로 곧바로 그 주소를 읽음. 해당하지 않으면 그대로 돌려줌.
func brSiteRewrite(norm string) string {
	u, err := url.Parse(norm)
	if err != nil {
		return norm
	}
	host := strings.ToLower(u.Hostname())
	if host != "blog.naver.com" && host != "m.blog.naver.com" {
		return norm
	}
	post := func(blogID, logNo string) string {
		if blogID == "" || logNo == "" || brAllDigits(logNo) == false {
			return norm
		}
		q := url.Values{"blogId": {blogID}, "logNo": {logNo}}
		return "https://blog.naver.com/PostView.naver?" + q.Encode()
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch {
	case len(segs) == 2 && brAllDigits(segs[1]): // /<아이디>/<글번호>
		return post(segs[0], segs[1])
	case len(segs) == 1 && strings.EqualFold(segs[0], "PostView.naver"), len(segs) == 1 && strings.EqualFold(segs[0], "PostView.nhn"):
		return norm
	case len(segs) == 1 && u.Query().Get("logNo") != "" && !strings.Contains(segs[0], "."): // /<아이디>?Redirect=Log&logNo=...
		return post(segs[0], u.Query().Get("logNo"))
	case len(segs) == 1 && strings.HasPrefix(strings.ToLower(segs[0]), "postlist") && u.Query().Get("blogId") != "" && u.Query().Get("logNo") != "":
		return post(u.Query().Get("blogId"), u.Query().Get("logNo"))
	}
	return norm
}

func brAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// brIsKnownBody 는 글 본문만 담고 있다고 알려진 영역인지: 네이버 블로그 스마트에디터(se-main-container)와 예전 에디터(postViewArea)
func brIsKnownBody(n *html.Node) bool {
	if brAttr(n, "id") == "postViewArea" {
		return true
	}
	for _, c := range strings.Fields(brAttr(n, "class")) {
		if c == "se-main-container" {
			return true
		}
	}
	return false
}

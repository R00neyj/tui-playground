package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Claude Code 기록 (F-08): ~/.claude/projects/**/*.jsonl
//
// 실제 기록으로 확인한 구조 (키 이름과 숫자 집계만 확인함):
//   - 응답 줄은 type=assistant 이고 message.usage 에 input_tokens, output_tokens,
//     cache_creation_input_tokens, cache_read_input_tokens (모두 정수), message.model, message.id,
//     줄 최상위에 timestamp(UTC, "…Z"), requestId 가 있음
//   - 같은 응답이 내용 블록마다 줄을 따로 남겨서 여러 줄로 중복 기록됨. 중복 줄은 message.id 와
//     requestId 가 같고 입력·캐시 토큰도 같지만 output_tokens 는 줄마다 커지다가 마지막 줄에서 최대가 됨.
//     그래서 (message.id, requestId) 를 키로 한 건으로 합치고 output_tokens 는 가장 큰 값, 시각은 가장 이른 값을 씀
//   - 모델이 "<synthetic>" 인 줄은 실제 응답이 아니라서 제외함

// claudeUsageDir 는 기록 폴더. 테스트에서 바꿔 끼움. CLAUDE_CONFIG_DIR 이 있으면 그 아래 projects 를 씀.
var claudeUsageDir = func() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "projects")
	}
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return ""
	}
	return filepath.Join(h, ".claude", "projects")
}

var claudeUsageSource = &usageSource{
	Name:          "Claude Code",
	EmptyHint:     "Claude Code 를 한 번이라도 쓰면 기록이 생김",
	Dir:           func() string { return claudeUsageDir() },
	Parse:         parseClaudeUsage,
	HasCacheWrite: true,
}

type claudeLine struct {
	Timestamp string `json:"timestamp"`
	RequestID string `json:"requestId"`
	Message   *struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			Input  int64 `json:"input_tokens"`
			Output int64 `json:"output_tokens"`
			CacheW int64 `json:"cache_creation_input_tokens"`
			CacheR int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

var claudeUsageMarker = []byte(`"output_tokens"`)

// parseClaudeUsage 는 파일 하나의 응답 기록을 읽어 파일 안에서 중복을 합친 목록을 돌려줌
func parseClaudeUsage(r io.Reader) ([]usageRec, error) {
	var recs []usageRec
	idx := map[string]int{}
	err := usageEachLine(r, func(line []byte) {
		if !bytes.Contains(line, claudeUsageMarker) {
			return
		}
		var l claudeLine
		if json.Unmarshal(line, &l) != nil || l.Message == nil || l.Message.Usage == nil {
			return
		}
		ts, err := time.Parse(time.RFC3339Nano, l.Timestamp)
		if err != nil || l.Message.Model == "" || l.Message.Model == "<synthetic>" {
			return
		}
		u := l.Message.Usage
		rec := usageRec{TS: ts, Model: l.Message.Model, In: u.Input, Out: u.Output, CacheW: u.CacheW, CacheR: u.CacheR}
		if l.Message.ID != "" {
			rec.Key = l.Message.ID + "|" + l.RequestID
			if i, dup := idx[rec.Key]; dup {
				p := &recs[i]
				p.Out, p.In = max(p.Out, rec.Out), max(p.In, rec.In)
				p.CacheW, p.CacheR = max(p.CacheW, rec.CacheW), max(p.CacheR, rec.CacheR)
				if rec.TS.Before(p.TS) {
					p.TS = rec.TS
				}
				return
			}
			idx[rec.Key] = len(recs)
		}
		recs = append(recs, rec)
	})
	return recs, err
}

// usageMaxLine 보다 긴 줄은 건너뜀 (도구 결과가 통째로 들어간 거대한 줄 방어)
const usageMaxLine = 64 << 20

// usageEachLine 은 줄 길이 제한 없이 한 줄씩 fn 에 넘김. fn 이 돌려받은 슬라이스는 호출이 끝나면 다시 쓰이므로 보관하면 안 됨.
func usageEachLine(r io.Reader, fn func([]byte)) error {
	br := bufio.NewReaderSize(r, 256<<10)
	var buf []byte
	skip := false
	for {
		chunk, err := br.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			if !skip {
				if len(buf)+len(chunk) > usageMaxLine {
					skip, buf = true, nil
				} else {
					buf = append(buf, chunk...)
				}
			}
			continue
		}
		switch {
		case skip:
			skip = false
		case len(buf) > 0:
			buf = append(buf, chunk...)
			fn(buf)
		case len(chunk) > 0:
			fn(chunk)
		}
		if cap(buf) > 8<<20 {
			buf = nil
		} else {
			buf = buf[:0]
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

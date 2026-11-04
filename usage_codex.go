package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Codex CLI 기록 (F-09): ~/.codex/sessions/YYYY/MM/DD/rollout-<시각>-<id>.jsonl
//
// 이 PC 에는 ~/.codex 가 없어서 실제 기록으로 확인하지 못함. 아래는 문서와 공개 소스에서 확인한 형식임:
//   - 기록 위치: CODEX_HOME 환경 변수가 있으면 그 아래, 없으면 ~/.codex 아래 sessions
//     (출처: openai/codex 소스와 agent-top-core 의 Codex 파서 docs.rs/crate/agent-top-core)
//   - 줄마다 최상위 timestamp(RFC 3339, UTC), type, payload
//   - type=turn_context 의 payload.model 이 모델 이름
//   - type=event_msg 이고 payload.type=token_count 인 줄의 payload.info 에
//     total_token_usage(세션 누적)와 last_token_usage(직전 응답 한 번). 각각
//     input_tokens, cached_input_tokens, output_tokens, reasoning_output_tokens, total_tokens
//     (출처: openai/codex codex-rs/protocol/src/protocol.rs 의 TokenUsage, TokenUsageInfo)
//   - input_tokens 는 cached_input_tokens 를 포함함 (소스의 non_cached_input() = input - cached).
//     그래서 입력 = input_tokens - cached_input_tokens, 캐시 읽기 = cached_input_tokens 로 나눠 겹치지 않게 함
//   - payload.info 가 null 인 줄(한도 정보만 있는 줄)은 건너뜀
//
// 같은 token_count 가 반복 기록될 수 있어서 last_token_usage 를 더하지 않고 total_token_usage 의 증가분을 더함.
// total 이 줄어들면(새 세션으로 이어진 경우) 그 값을 새 기준으로 보고 total 전체를 증가분으로 봄.
// total_token_usage 가 없는 줄은 last_token_usage 를 그대로 더함.
//
// 불확실한 점: output_tokens 에 reasoning_output_tokens 가 포함되는지 문서에 적혀 있지 않아 output_tokens 만 씀.
// 이어받은(fork) 세션이 이전 줄을 복사하면 두 번 셀 수 있음. Codex 는 캐시 생성 토큰을 따로 기록하지 않아 표에 - 로 표시함.

// codexUsageDir 는 기록 폴더. 테스트에서 바꿔 끼움.
var codexUsageDir = func() string {
	if d := os.Getenv("CODEX_HOME"); d != "" {
		return filepath.Join(d, "sessions")
	}
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return ""
	}
	return filepath.Join(h, ".codex", "sessions")
}

var codexUsageSource = &usageSource{
	Name:          "Codex",
	EmptyHint:     "Codex CLI 를 한 번이라도 쓰면 기록이 생김",
	Dir:           func() string { return codexUsageDir() },
	Parse:         parseCodexUsage,
	HasCacheWrite: false,
}

type codexTokens struct {
	Input  int64 `json:"input_tokens"`
	Cached int64 `json:"cached_input_tokens"`
	Output int64 `json:"output_tokens"`
}

type codexLine struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Type  string `json:"type"`
		Model string `json:"model"`
		Info  *struct {
			Total *codexTokens `json:"total_token_usage"`
			Last  *codexTokens `json:"last_token_usage"`
		} `json:"info"`
	} `json:"payload"`
}

var (
	codexMarkerCount   = []byte(`"token_count"`)
	codexMarkerContext = []byte(`"turn_context"`)
)

// parseCodexUsage 는 세션 파일 하나에서 token_count 마다 토큰 증가분 한 건을 만듦
func parseCodexUsage(r io.Reader) ([]usageRec, error) {
	var recs []usageRec
	var prev codexTokens
	model := "(모델 모름)"
	err := usageEachLine(r, func(line []byte) {
		isCount := bytes.Contains(line, codexMarkerCount)
		if !isCount && !bytes.Contains(line, codexMarkerContext) {
			return
		}
		var l codexLine
		if json.Unmarshal(line, &l) != nil {
			return
		}
		switch {
		case l.Type == "turn_context" && l.Payload.Model != "":
			model = l.Payload.Model
		case l.Type == "event_msg" && l.Payload.Type == "token_count" && l.Payload.Info != nil:
			ts, err := time.Parse(time.RFC3339Nano, l.Timestamp)
			if err != nil {
				return
			}
			var d codexTokens
			switch info := l.Payload.Info; {
			case info.Total != nil:
				t := *info.Total
				d = codexTokens{t.Input - prev.Input, t.Cached - prev.Cached, t.Output - prev.Output}
				if d.Input < 0 || d.Cached < 0 || d.Output < 0 {
					d = t
				}
				prev = t
			case info.Last != nil:
				d = *info.Last
			default:
				return
			}
			if d == (codexTokens{}) {
				return
			}
			recs = append(recs, usageRec{
				TS: ts, Model: model,
				In: max(0, d.Input-d.Cached), Out: d.Output, CacheR: d.Cached,
			})
		}
	})
	return recs, err
}

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// 미니게임 최고 기록. 사용자 설정 폴더의 TUI-playground/scores.json 에 {"이름": 값} 으로 저장함.
// 값의 의미(클수록 좋음/작을수록 좋음)는 쓰는 앱이 정함. 파일을 못 읽거나 못 써도 게임은 그대로 됨.

// scoresPath 는 테스트에서 임시 폴더로 바꿔 끼움 (TestMain)
var scoresPath = func() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, "TUI-playground", "scores.json")
}

func loadScores() map[string]int {
	m := map[string]int{}
	p := scoresPath()
	if p == "" {
		return m
	}
	if data, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(data, &m)
	}
	return m
}

// loadScore 는 기록이 있으면 (값, true)
func loadScore(name string) (int, bool) {
	v, ok := loadScores()[name]
	return v, ok
}

// saveScore 는 다른 기록을 지우지 않게 파일 전체를 읽고 한 항목만 바꿔서 임시 파일에 쓴 뒤 교체함
func saveScore(name string, v int) error {
	p := scoresPath()
	if p == "" {
		return os.ErrNotExist
	}
	m := loadScores()
	m[name] = v
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

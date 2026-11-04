package main

// Claude Code 사용량 (F-08). 화면과 읽기 로직은 usage_view.go, 기록 형식은 usage_claude.go 에 있음.
var claudeUsageInfo = AppInfo{
	ID:   "claude-usage",
	Name: "Claude 사용량",
	Desc: "Claude Code 토큰 사용 기록",
	New:  func(env *Env) App { return newUsageApp(claudeUsageSource) },
}

package main

// 자리표시 구현. 이 파일을 통째로 실제 앱으로 바꿔서 쓴다 (AppInfo 변수 이름은 유지할 것).
var stopwatchInfo = AppInfo{
	ID:   "stopwatch",
	Name: "스톱워치",
	Desc: "시간 측정과 랩 기록",
	New:  func(env *Env) App { return newStub("스톱워치") },
}

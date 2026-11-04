package main

// 자리표시 구현. 이 파일을 통째로 실제 앱으로 바꿔서 쓴다 (AppInfo 변수 이름은 유지할 것).
var pomodoroInfo = AppInfo{
	ID:   "pomodoro",
	Name: "포모도로",
	Desc: "집중·휴식 반복 카운트다운",
	New:  func(env *Env) App { return newStub("포모도로") },
}

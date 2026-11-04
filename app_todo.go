package main

// 자리표시 구현. 이 파일을 통째로 실제 앱으로 바꿔서 쓴다 (AppInfo 변수 이름은 유지할 것).
var todoInfo = AppInfo{
	ID:   "todo",
	Name: "할 일",
	Desc: "간단한 할 일 목록",
	New:  func(env *Env) App { return newStub("할 일") },
}

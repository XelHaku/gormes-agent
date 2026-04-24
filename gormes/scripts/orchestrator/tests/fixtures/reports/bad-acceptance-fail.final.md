1) Selected task
Task: 1 / 1.A / Item A2

2) Pre-doc baseline
Files:
- docs/progress.json

3) RED proof
Command: go test ./internal/foo
Exit: 1
Snippet: FAIL: TestBar

4) GREEN proof
Command: go test ./internal/foo
Exit: 0
Snippet: PASS

5) REFACTOR proof
Command: go test ./internal/foo
Exit: 0
Snippet: PASS

6) Regression proof
Command: go test ./...
Exit: 0
Snippet: ok

7) Post-doc closeout
Files:
- docs/progress.json

8) Commit
Branch: codexu/test-run/worker1
Commit: abc1234def5678
Files:
- internal/foo/foo.go

9) Acceptance check
Criterion: TestBar fails before implementation — PASS
Criterion: TestBar passes after implementation — PASS
Criterion: no breaking change to the existing Handler interface — FAIL: I had to rename Run() to Execute()

---
schema: 1
id: E-bab35e7a-7b7f-4644-b6ec-ad6e3adf90de
subject: D-d123fcee-1824-40fb-b5f6-15fe91d6ee8f
subject_digest: sha256:5ac1bf0196149865fe779d3415e0a6129bd01b12c551fbfde834a1575477f5af
kind: regression-test
trust: local
target:
  commit: ""
  content_digest: sha256:d42b5df3a1e419f951e973c3c88e22e6a46872354f977f9f6ab022018c37c015
  dirty_inputs:
    - Makefile
    - install.ps1
    - install.sh
    - internal/cli/archive.go
    - internal/cli/args.go
    - internal/cli/body.go
    - internal/cli/brief.go
    - internal/cli/check.go
    - internal/cli/cli.go
    - internal/cli/complete.go
    - internal/cli/complete_test.go
    - internal/cli/completion.go
    - internal/cli/decide.go
    - internal/cli/hook.go
    - internal/cli/init.go
    - internal/cli/install.go
    - internal/cli/install_test.go
    - internal/cli/note.go
    - internal/cli/promote.go
    - internal/cli/review.go
    - internal/cli/script_test.go
    - internal/cli/shell.go
    - internal/cli/spec.go
    - internal/cli/spec_test.go
    - internal/cli/sync.go
    - internal/cli/task.go
    - internal/cli/template.go
    - internal/cli/testdata/script/commit-msg-trailer.txtar
    - internal/cli/testdata/script/completion.txtar
    - internal/cli/testdata/script/decide-accepted-sections.txtar
    - internal/cli/testdata/script/decide-basic.txtar
    - internal/cli/testdata/script/gate-dependency.txtar
    - internal/cli/testdata/script/gate-index-vs-worktree.txtar
    - internal/cli/testdata/script/hook-protocol.txtar
    - internal/cli/testdata/script/init-sync.txtar
    - internal/cli/testdata/script/knowledge-index-layers.txtar
    - internal/cli/testdata/script/knowledge-index.txtar
    - internal/cli/testdata/script/memory-archive.txtar
    - internal/cli/testdata/script/memory-evidence-stale.txtar
    - internal/cli/testdata/script/memory-lifecycle.txtar
    - internal/cli/testdata/script/note-candidate.txtar
    - internal/cli/testdata/script/object-integrity.txtar
    - internal/cli/testdata/script/policy-no-auto-promote.txtar
    - internal/cli/testdata/script/recall-across-tools.txtar
    - internal/cli/testdata/script/review-archive-candidates.txtar
    - internal/cli/testdata/script/review-clusters.txtar
    - internal/cli/testdata/script/rule-candidate-not-executed.txtar
    - internal/cli/testdata/script/rule-exec-outcomes.txtar
    - internal/cli/testdata/script/rule-promote-rejected.txtar
    - internal/cli/testdata/script/rule-promote.txtar
    - internal/cli/testdata/script/rule-retire-and-review.txtar
    - internal/cli/testdata/script/rule-scope-gating.txtar
    - internal/cli/testdata/script/stop-does-not-run-rules.txtar
    - internal/cli/testdata/script/supersede-accepted-migrates.txtar
    - internal/cli/testdata/script/supersede-proposed-keeps-old.txtar
    - internal/cli/testdata/script/sync-codemap.txtar
    - internal/cli/testdata/script/sync-ownership.txtar
    - internal/cli/testdata/script/task-continuity.txtar
    - internal/cli/testdata/script/template-import.txtar
    - internal/cli/testdata/script/template-update.txtar
    - internal/cli/testdata/script/usage-errors.txtar
    - internal/cli/testdata/script/verify-decision-keeps-status.txtar
    - internal/cli/testdata/script/verify-fail-and-error.txtar
    - internal/cli/testdata/script/verify-produces-evidence.txtar
    - internal/cli/testdata/script/version.txtar
    - internal/cli/update.go
    - internal/cli/update_test.go
    - internal/cli/verify.go
    - internal/cli/why.go
    - internal/selfupdate/selfupdate.go
    - internal/selfupdate/selfupdate_test.go
    - internal/ui/progress.go
verifier:
  id: go-test
  definition_digest: sha256:4db15ecce6bf44a42e77fed07adf40bd272128c373f0bf35a78e44a946d504f6
  command:
    - go
    - test
    - ./...
result: pass
observed_at: "2026-09-11T03:26:16Z"
producer: keel-verify
---

验证对象：D-d123fcee-1824-40fb-b5f6-15fe91d6ee8f 补全与自更新：spec 一处来源 + 校验和必对

结果：pass（退出码 0）

```
?   	github.com/shiftu/keel/cmd/keel	[no test files]
?   	github.com/shiftu/keel/internal/adapter	[no test files]
?   	github.com/shiftu/keel/internal/brief	[no test files]
ok  	github.com/shiftu/keel/internal/check	(cached)
ok  	github.com/shiftu/keel/internal/cli	(cached)
ok  	github.com/shiftu/keel/internal/gitx	(cached)
ok  	github.com/shiftu/keel/internal/manifest	(cached)
ok  	github.com/shiftu/keel/internal/policy	(cached)
?   	github.com/shiftu/keel/internal/render	[no test files]
?   	github.com/shiftu/keel/internal/review	[no test files]
ok  	github.com/shiftu/keel/internal/selfupdate	(cached)
ok  	github.com/shiftu/keel/internal/store	(cached)
?   	github.com/shiftu/keel/internal/template	[no test files]
?   	github.com/shiftu/keel/internal/ui	[no test files]
?   	github.com/shiftu/keel/internal/verify	[no test files]
?   	github.com/shiftu/keel/templates	[no test files]
```

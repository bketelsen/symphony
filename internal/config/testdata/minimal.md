---
tracker:
  repo: owner/repo
  active_states:
    - "symphony:todo"

workspace:
  root: /tmp/ws
  repo_url: git@github.com:owner/repo.git
---

Work on {{ .Issue.Title }}.

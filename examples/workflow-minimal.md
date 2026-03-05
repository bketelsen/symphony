---
tracker:
  kind: github
  repo: owner/repo
  active_states:
    - "symphony:todo"
  terminal_states:
    - "symphony:done"

workspace:
  root: /tmp/symphony-workspaces
  repo_url: git@github.com:owner/repo.git
  base_branch: main

claude:
  command: claude --print
---

You are working on issue {{ .Issue.Identifier }}: {{ .Issue.Title }}

{{ deref .Issue.Description }}

When your work is complete, create a draft pull request using:
  gh pr create --draft --title "{{ .Issue.Title }}" --body "Closes #{{ .Issue.Number }}"

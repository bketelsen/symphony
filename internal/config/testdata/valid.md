---
tracker:
  kind: github
  repo: myorg/myrepo
  api_key: test-token
  active_states:
    - "symphony:todo"
    - "symphony:in-progress"
  terminal_states:
    - "symphony:done"
    - "symphony:cancelled"

polling:
  interval_ms: 15000

workspace:
  root: /tmp/workspaces
  repo_url: git@github.com:myorg/myrepo.git
  base_branch: develop

hooks:
  after_create: |
    npm install
  before_run: |
    git pull
  timeout_ms: 30000

agent:
  max_concurrent_agents: 3
  max_turns: 10
  max_retry_backoff_ms: 120000

claude:
  command: claude --print
  model: sonnet
  max_tokens: 8000
  turn_timeout_ms: 1800000
  stall_timeout_ms: 120000
  allowed_tools:
    - bash
    - read
  permission_mode: auto

server:
  port: 9090
---

You are working on issue {{ .Issue.Identifier }}: {{ .Issue.Title }}

Fix the problem described above.

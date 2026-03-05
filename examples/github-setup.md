# GitHub Setup Guide

This guide covers the GitHub configuration needed to use symphony effectively.

## Required Labels

Symphony uses GitHub labels to track issue state. The following labels are required:

| Label | Color | Purpose |
|-------|-------|---------|
| `symphony:todo` | green | Issues ready for agents to pick up |
| `symphony:in-progress` | yellow | Currently being worked on (set automatically by symphony) |
| `symphony:done` | blue | Completed (set automatically when PR is merged, via hooks) |
| `symphony:cancelled` | gray | Will not be worked on |

## Priority Labels (Optional)

Symphony supports priority labels for ordering work. Lower number = higher priority. Issues without a priority label sort last.

| Label | Color | Priority |
|-------|-------|----------|
| `P0` | red | Critical — highest priority |
| `P1` | yellow | High |
| `P2` | blue | Medium |
| `P3` | — | Low |
| `P4` | — | Lowest priority |

## Creating Labels

### Option A: Run `symphony init`

The easiest approach — `symphony init` automatically creates all required labels (and optional priority labels P0–P2) in your repository:

```bash
symphony init
```

### Option B: Use `gh` CLI manually

Copy and paste the following commands to create all labels:

```bash
# Required labels
gh label create "symphony:todo"        --color "0e8a16" --description "Ready for agents to pick up"
gh label create "symphony:in-progress" --color "fbca04" --description "Currently being worked on"
gh label create "symphony:done"        --color "0075ca" --description "Completed"
gh label create "symphony:cancelled"   --color "cccccc" --description "Will not be worked on"

# Priority labels (optional)
gh label create "P0" --color "d93f0b" --description "Critical priority"
gh label create "P1" --color "fbca04" --description "High priority"
gh label create "P2" --color "0075ca" --description "Medium priority"
```

## Issue Template

Create `.github/ISSUE_TEMPLATE/symphony-task.md` to provide a consistent structure for symphony issues:

```markdown
---
name: Symphony Task
about: A task for symphony agents to work on
title: ''
labels: 'symphony:todo'
assignees: ''
---

## What to build

<!-- Describe the feature, fix, or task in detail. Be specific about requirements. -->

## Files

<!-- List key files to create or modify -->

## Acceptance Criteria

<!-- Optional: list specific outcomes that define done -->
```

To add a priority label, apply one of `P0`, `P1`, `P2`, etc. alongside `symphony:todo` when creating the issue.

## Branch Protection

Recommended branch protection settings for your default branch:

- **Require pull request reviews** — ensures agent work is reviewed before merging
- **Require status checks to pass** — run CI (tests, lint) before merge
- **Require branches to be up to date** — prevents stale merges

To configure via `gh`:

```bash
gh api repos/{owner}/{repo}/branches/main/protection \
  --method PUT \
  --field required_status_checks='{"strict":true,"contexts":["test"]}' \
  --field enforce_admins=false \
  --field required_pull_request_reviews='{"required_approving_review_count":1}' \
  --field restrictions=null
```

Adjust `contexts` to match your CI workflow job names.

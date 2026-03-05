package planner

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestParsePlan_BasicPlan(t *testing.T) {
	plan, err := ParsePlan(`# My Feature

**Goal:** Build a thing.

**Architecture:** Simple and clean.

---

### Task 1: First Component

**Files:**
- Create: ` + "`internal/foo.go`" + `
- Modify: ` + "`internal/bar.go`" + `

**What to build:**

Implement the foo module with bar integration.

**Commit:** ` + "`feat: add foo module`" + `

---

### Task 2: Second Component

**Files:**
- Create: ` + "`internal/baz.go`" + `

**Depends on:** Task 1

**What to build:**

Build baz on top of foo.

**Commit:** ` + "`feat: add baz module`" + `

---

### Task 3: Integration

**Depends on:** Task 1, Task 2

**What to build:**

Wire everything together.

**Commit:** ` + "`feat: integrate modules`" + `

---
`)
	if err != nil {
		t.Fatal(err)
	}

	if plan.Title != "My Feature" {
		t.Errorf("Title = %q, want %q", plan.Title, "My Feature")
	}
	if plan.Goal != "Build a thing." {
		t.Errorf("Goal = %q, want %q", plan.Goal, "Build a thing.")
	}
	if plan.Architecture != "Simple and clean." {
		t.Errorf("Architecture = %q, want %q", plan.Architecture, "Simple and clean.")
	}
	if len(plan.Tasks) != 3 {
		t.Fatalf("got %d tasks, want 3", len(plan.Tasks))
	}

	// Task 1
	t1 := plan.Tasks[0]
	if t1.Number != 1 {
		t.Errorf("Task 1 Number = %d", t1.Number)
	}
	if t1.Title != "First Component" {
		t.Errorf("Task 1 Title = %q", t1.Title)
	}
	if len(t1.FilesCreate) != 1 || t1.FilesCreate[0] != "internal/foo.go" {
		t.Errorf("Task 1 FilesCreate = %v", t1.FilesCreate)
	}
	if len(t1.FilesModify) != 1 || t1.FilesModify[0] != "internal/bar.go" {
		t.Errorf("Task 1 FilesModify = %v", t1.FilesModify)
	}
	if t1.CommitMsg != "feat: add foo module" {
		t.Errorf("Task 1 CommitMsg = %q", t1.CommitMsg)
	}
	if len(t1.DependsOn) != 0 {
		t.Errorf("Task 1 DependsOn = %v, want empty", t1.DependsOn)
	}

	// Task 2
	t2 := plan.Tasks[1]
	if t2.Number != 2 {
		t.Errorf("Task 2 Number = %d", t2.Number)
	}
	if len(t2.DependsOn) != 1 || t2.DependsOn[0] != 1 {
		t.Errorf("Task 2 DependsOn = %v, want [1]", t2.DependsOn)
	}

	// Task 3
	t3 := plan.Tasks[2]
	if len(t3.DependsOn) != 2 || t3.DependsOn[0] != 1 || t3.DependsOn[1] != 2 {
		t.Errorf("Task 3 DependsOn = %v, want [1, 2]", t3.DependsOn)
	}
}

func TestParsePlan_MissingTitle(t *testing.T) {
	_, err := ParsePlan("No title here\n### Task 1: Something\n")
	if err == nil {
		t.Fatal("expected error for missing H1 title")
	}
}

func TestParsePlan_ForwardDependency(t *testing.T) {
	_, err := ParsePlan(`# Plan

### Task 1: First

**Depends on:** Task 3

**What to build:**
Something.

---

### Task 2: Second

**What to build:**
Something.

---

### Task 3: Third

**What to build:**
Something.
`)
	if err == nil {
		t.Fatal("expected error for forward dependency")
	}
}

func TestParsePlan_NoTasks(t *testing.T) {
	plan, err := ParsePlan(`# Empty Plan

**Goal:** Nothing to do.

Some preamble text.
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Tasks) != 0 {
		t.Errorf("got %d tasks, want 0", len(plan.Tasks))
	}
}

func TestParsePlan_BodyContent(t *testing.T) {
	plan, err := ParsePlan(`# Plan

### Task 1: Implement Widget

**What to build:**

Create the widget struct and validation.
Include JSON serialization.

**Commit:** ` + "`feat: widget`" + `

---
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Tasks) != 1 {
		t.Fatalf("got %d tasks", len(plan.Tasks))
	}
	body := plan.Tasks[0].Body
	if body == "" {
		t.Fatal("empty body")
	}
	if !contains(body, "widget struct") {
		t.Errorf("body missing expected content: %q", body)
	}
	if !contains(body, "JSON serialization") {
		t.Errorf("body missing expected content: %q", body)
	}
}

func TestParseFile_SamplePlan(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	testdata := filepath.Join(filepath.Dir(thisFile), "testdata", "sample_plan.md")

	plan, err := ParseFile(testdata)
	if err != nil {
		t.Fatal(err)
	}

	if plan.Title != "Sample Feature Implementation Plan" {
		t.Errorf("Title = %q", plan.Title)
	}
	if len(plan.Tasks) != 3 {
		t.Fatalf("got %d tasks, want 3", len(plan.Tasks))
	}

	// Task 1: no dependencies
	if len(plan.Tasks[0].DependsOn) != 0 {
		t.Errorf("Task 1 DependsOn = %v", plan.Tasks[0].DependsOn)
	}
	if len(plan.Tasks[0].FilesCreate) != 2 {
		t.Errorf("Task 1 FilesCreate = %v", plan.Tasks[0].FilesCreate)
	}

	// Task 2: depends on Task 1
	if len(plan.Tasks[1].DependsOn) != 1 || plan.Tasks[1].DependsOn[0] != 1 {
		t.Errorf("Task 2 DependsOn = %v", plan.Tasks[1].DependsOn)
	}

	// Task 3: depends on Task 1 and Task 2
	if len(plan.Tasks[2].DependsOn) != 2 {
		t.Errorf("Task 3 DependsOn = %v", plan.Tasks[2].DependsOn)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

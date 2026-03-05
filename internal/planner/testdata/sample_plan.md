# Sample Feature Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a new widget system to the application.

**Architecture:** Three-layer design with domain types, service layer, and HTTP handlers.

---

### Task 1: Domain Types

**Files:**
- Create: `internal/widget/types.go`
- Create: `internal/widget/types_test.go`

**What to build:**

Define Widget struct with ID, Name, and Config fields. Add validation method.

**Tests:**
- Valid widget passes validation
- Empty name fails validation

**Commit:** `feat: add widget domain types`

---

### Task 2: Widget Service

**Files:**
- Create: `internal/widget/service.go`
- Modify: `internal/app/app.go`

**Depends on:** Task 1

**What to build:**

Create WidgetService with CRUD operations. Wire into app struct.

**Tests:**
- Create widget returns ID
- Get widget by ID

**Commit:** `feat: add widget service`

---

### Task 3: HTTP Handlers

**Files:**
- Create: `internal/widget/handler.go`
- Modify: `internal/app/routes.go`

**Depends on:** Task 1, Task 2

**What to build:**

REST endpoints: GET /widgets, POST /widgets, GET /widgets/:id.

**Tests:**
- GET /widgets returns 200
- POST /widgets creates widget

**Commit:** `feat: add widget HTTP handlers`

---

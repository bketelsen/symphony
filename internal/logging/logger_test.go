package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
)

func TestSetupWritesValidJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := SetupWithWriter(&buf, slog.LevelInfo)

	logger.Info("test message", "key", "value")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if entry["msg"] != "test message" {
		t.Errorf("msg = %v, want %q", entry["msg"], "test message")
	}
	if entry["key"] != "value" {
		t.Errorf("key = %v, want %q", entry["key"], "value")
	}
}

func TestWithIssue(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := SetupWithWriter(&buf, slog.LevelInfo)
	logger = WithIssue(logger, "I_abc", "owner/repo#1")

	logger.Info("issue log")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if entry["issue_id"] != "I_abc" {
		t.Errorf("issue_id = %v, want %q", entry["issue_id"], "I_abc")
	}
	if entry["issue_identifier"] != "owner/repo#1" {
		t.Errorf("issue_identifier = %v, want %q", entry["issue_identifier"], "owner/repo#1")
	}
}

func TestWithSession(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := SetupWithWriter(&buf, slog.LevelInfo)
	logger = WithSession(logger, "sess-42")

	logger.Info("session log")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if entry["session_id"] != "sess-42" {
		t.Errorf("session_id = %v, want %q", entry["session_id"], "sess-42")
	}
}

func TestCombinedEnrichment(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := SetupWithWriter(&buf, slog.LevelInfo)
	logger = WithSession(logger, "s1")
	logger = WithIssue(logger, "id1", "repo#1")

	logger.Info("combined")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if entry["session_id"] != "s1" {
		t.Errorf("session_id = %v, want %q", entry["session_id"], "s1")
	}
	if entry["issue_id"] != "id1" {
		t.Errorf("issue_id = %v, want %q", entry["issue_id"], "id1")
	}
	if entry["issue_identifier"] != "repo#1" {
		t.Errorf("issue_identifier = %v, want %q", entry["issue_identifier"], "repo#1")
	}
}

func TestErrorAttr(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := SetupWithWriter(&buf, slog.LevelInfo)

	attr := ErrorAttr(fmt.Errorf("something went wrong"))
	logger.Info("error log", attr)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if entry["error"] != "something went wrong" {
		t.Errorf("error = %v, want %q", entry["error"], "something went wrong")
	}
}

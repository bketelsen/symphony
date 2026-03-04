package logging

import (
	"io"
	"log/slog"
	"os"
)

// Setup initializes and returns a structured JSON logger.
func Setup(level slog.Level) *slog.Logger {
	return SetupWithWriter(os.Stderr, level)
}

// SetupWithWriter creates a logger writing to the given writer.
func SetupWithWriter(w io.Writer, level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(handler)
}

// WithIssue returns a logger enriched with issue context fields.
func WithIssue(logger *slog.Logger, issueID, identifier string) *slog.Logger {
	return logger.With("issue_id", issueID, "issue_identifier", identifier)
}

// WithSession returns a logger enriched with a session ID.
func WithSession(logger *slog.Logger, sessionID string) *slog.Logger {
	return logger.With("session_id", sessionID)
}

// ErrorAttr creates a slog attribute from an error.
func ErrorAttr(err error) slog.Attr {
	return slog.String("error", err.Error())
}

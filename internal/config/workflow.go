package config

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// envVarPattern matches $VAR and ${VAR} references.
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// ParseWorkflow parses a WORKFLOW.md file from a reader.
// Returns the typed config, the prompt template body, and any error.
func ParseWorkflow(r io.Reader) (*Config, string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, "", fmt.Errorf("config: read workflow: %w", err)
	}

	content := string(data)
	yamlBlock, prompt, err := splitFrontMatter(content)
	if err != nil {
		return nil, "", err
	}

	// Expand environment variables in YAML block
	yamlBlock = expandEnvVars(yamlBlock)

	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlBlock), &cfg); err != nil {
		return nil, "", fmt.Errorf("config: parse YAML: %w", err)
	}

	cfg.ApplyDefaults()

	return &cfg, strings.TrimSpace(prompt), nil
}

// ParseWorkflowFile parses a WORKFLOW.md file from a filesystem path.
func ParseWorkflowFile(path string) (*Config, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("config: open workflow file: %w", err)
	}
	defer f.Close()
	return ParseWorkflow(f)
}

// splitFrontMatter splits content on "---" fences into YAML and prompt body.
func splitFrontMatter(content string) (yaml string, prompt string, err error) {
	// Trim leading whitespace/newlines
	content = strings.TrimLeft(content, " \t\r\n")

	if !strings.HasPrefix(content, "---") {
		return "", "", fmt.Errorf("config: workflow must start with ---")
	}

	// Find the closing ---
	rest := content[3:] // skip opening ---
	rest = strings.TrimLeft(rest, " \t")
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", fmt.Errorf("config: missing closing --- in front matter")
	}

	yamlBlock := rest[:idx]
	afterClosing := rest[idx+4:] // skip \n---

	// Trim the line after closing ---
	if len(afterClosing) > 0 && afterClosing[0] == '\n' {
		afterClosing = afterClosing[1:]
	} else if len(afterClosing) > 1 && afterClosing[0] == '\r' && afterClosing[1] == '\n' {
		afterClosing = afterClosing[2:]
	}

	return yamlBlock, afterClosing, nil
}

// expandEnvVars replaces $VAR and ${VAR} with environment variable values.
func expandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name from either ${VAR} or $VAR
		var name string
		if strings.HasPrefix(match, "${") {
			name = match[2 : len(match)-1]
		} else {
			name = match[1:]
		}
		return os.Getenv(name)
	})
}

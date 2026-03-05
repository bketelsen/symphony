package planner

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	h1Re       = regexp.MustCompile(`^#\s+(.+)$`)
	taskRe     = regexp.MustCompile(`^###\s+Task\s+(\d+):\s+(.+)$`)
	fileLineRe = regexp.MustCompile(`^-\s+(Create|Modify):\s+` + "`" + `([^` + "`" + `]+)` + "`")
	dependsRe  = regexp.MustCompile(`(?i)Task\s+(\d+)`)
	commitRe   = regexp.MustCompile("`" + `([^` + "`" + `]+)` + "`")
	goalRe     = regexp.MustCompile(`^\*\*Goal:\*\*\s*(.+)$`)
	archRe     = regexp.MustCompile(`^\*\*Architecture:\*\*\s*(.+)$`)
)

// ParseFile reads a plan file and parses it.
func ParseFile(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("planner: read file: %w", err)
	}
	return ParsePlan(string(data))
}

// ParsePlan parses a markdown implementation plan into structured tasks.
func ParsePlan(content string) (*Plan, error) {
	lines := strings.Split(content, "\n")

	plan := &Plan{}

	// Extract H1 title
	for _, line := range lines {
		if m := h1Re.FindStringSubmatch(line); m != nil {
			plan.Title = strings.TrimSpace(m[1])
			break
		}
	}
	if plan.Title == "" {
		return nil, fmt.Errorf("planner: missing H1 title (# Title)")
	}

	// Extract Goal and Architecture from preamble
	for _, line := range lines {
		if m := goalRe.FindStringSubmatch(line); m != nil {
			plan.Goal = strings.TrimSpace(m[1])
		}
		if m := archRe.FindStringSubmatch(line); m != nil {
			plan.Architecture = strings.TrimSpace(m[1])
		}
	}

	// Find all task section boundaries
	type taskStart struct {
		lineIndex int
		number    int
		title     string
	}
	var starts []taskStart
	for i, line := range lines {
		if m := taskRe.FindStringSubmatch(line); m != nil {
			num, _ := strconv.Atoi(m[1])
			starts = append(starts, taskStart{
				lineIndex: i,
				number:    num,
				title:     strings.TrimSpace(m[2]),
			})
		}
	}

	// Parse each task section
	for i, ts := range starts {
		endLine := len(lines)
		if i+1 < len(starts) {
			endLine = starts[i+1].lineIndex
		}

		section := lines[ts.lineIndex+1 : endLine]
		task := parseTaskSection(ts.number, ts.title, section)
		plan.Tasks = append(plan.Tasks, task)
	}

	// Validate: no forward dependencies
	for _, task := range plan.Tasks {
		for _, dep := range task.DependsOn {
			if dep >= task.Number {
				return nil, fmt.Errorf("planner: Task %d depends on Task %d, but dependencies must reference earlier tasks", task.Number, dep)
			}
		}
	}

	return plan, nil
}

func parseTaskSection(number int, title string, lines []string) Task {
	task := Task{
		Number: number,
		Title:  title,
	}

	var bodyLines []string
	var inFilesBlock bool
	var inDependsLine bool

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Section delimiter
		if trimmed == "---" {
			break
		}

		// Detect sub-headings
		if strings.HasPrefix(trimmed, "**Files:**") {
			inFilesBlock = true
			inDependsLine = false
			continue
		}
		if strings.HasPrefix(trimmed, "**Depends on:**") {
			inFilesBlock = false
			inDependsLine = true
			// Parse dependencies from this line
			matches := dependsRe.FindAllStringSubmatch(trimmed, -1)
			for _, m := range matches {
				num, _ := strconv.Atoi(m[1])
				task.DependsOn = append(task.DependsOn, num)
			}
			continue
		}
		if strings.HasPrefix(trimmed, "**Commit:**") {
			inFilesBlock = false
			inDependsLine = false
			if m := commitRe.FindStringSubmatch(trimmed); m != nil {
				task.CommitMsg = m[1]
			}
			continue
		}
		if strings.HasPrefix(trimmed, "**") && strings.Contains(trimmed, ":**") {
			inFilesBlock = false
			inDependsLine = false
		}

		// Parse file lines
		if inFilesBlock {
			if m := fileLineRe.FindStringSubmatch(trimmed); m != nil {
				switch m[1] {
				case "Create":
					task.FilesCreate = append(task.FilesCreate, m[2])
				case "Modify":
					task.FilesModify = append(task.FilesModify, m[2])
				}
				continue
			}
		}

		// Parse continuation depends lines
		if inDependsLine && strings.HasPrefix(trimmed, "-") {
			matches := dependsRe.FindAllStringSubmatch(trimmed, -1)
			for _, m := range matches {
				num, _ := strconv.Atoi(m[1])
				task.DependsOn = append(task.DependsOn, num)
			}
			continue
		}

		// Accumulate body lines (skip empty leading lines)
		if len(bodyLines) > 0 || trimmed != "" {
			bodyLines = append(bodyLines, line)
		}
	}

	// Trim trailing empty lines from body
	for len(bodyLines) > 0 && strings.TrimSpace(bodyLines[len(bodyLines)-1]) == "" {
		bodyLines = bodyLines[:len(bodyLines)-1]
	}
	task.Body = strings.Join(bodyLines, "\n")

	return task
}

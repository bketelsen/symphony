package planner

// Plan represents a parsed implementation plan.
type Plan struct {
	Title        string
	Goal         string
	Architecture string
	Tasks        []Task
}

// Task represents a single task extracted from the plan.
type Task struct {
	Number      int
	Title       string
	FilesCreate []string
	FilesModify []string
	Body        string // raw section content for issue body
	DependsOn   []int  // task numbers this depends on
	CommitMsg   string
}

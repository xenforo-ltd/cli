package build

// Task identifies a stage in the release build pipeline.
type Task string

// Release build stages, reported only when enabled for the current build.
const (
	TaskComposer Task = "composer"
	TaskExport   Task = "export"
	TaskRollup   Task = "rollup"
	TaskMinify   Task = "minify"
	TaskExec     Task = "exec"
	TaskHashes   Task = "hashes"
	TaskArchive  Task = "archive"
)

// WithTaskProgress registers a callback before each task, including file
// collection. The returned function, if non-nil, is called when the task finishes
// with its error, or nil on success. Callbacks run synchronously in task order.
func WithTaskProgress(progress func(Task) func(error)) Option {
	return func(o *buildOptions) {
		o.taskProgress = progress
	}
}

// taskKind maps each pipeline implementation to its public progress identifier.
func taskKind(t task) Task {
	switch t.(type) {
	case composerTask:
		return TaskComposer
	case exportDataTask:
		return TaskExport
	case rollupTask:
		return TaskRollup
	case minifyTask:
		return TaskMinify
	case execTask:
		return TaskExec
	case *hashesTask:
		return TaskHashes
	case *zipTask:
		return TaskArchive
	default:
		return ""
	}
}

package build

// Task identifies a stage in the release build pipeline.
type Task string

const (
	// TaskComposer identifies dependency installation.
	TaskComposer Task = "composer"
	// TaskExport identifies development-output export.
	TaskExport Task = "export"
	// TaskRollup identifies JavaScript bundling.
	TaskRollup Task = "rollup"
	// TaskMinify identifies JavaScript minification.
	TaskMinify Task = "minify"
	// TaskExec identifies configured command execution.
	TaskExec Task = "exec"
	// TaskHashes identifies file selection and hash manifest creation.
	TaskHashes Task = "hashes"
	// TaskArchive identifies release archive creation.
	TaskArchive Task = "archive"
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

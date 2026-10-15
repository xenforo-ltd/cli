package main

import (
	"context"
	"io"
	"sync"

	"github.com/xenforo-ltd/cli/internal/build"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// buildProgress presents each build stage without mixing spinner frames with
// subprocess output or warnings. mu also serializes subprocess stdout and stderr.
type buildProgress struct {
	mu      sync.Mutex
	ctx     context.Context
	verbose bool
	spinner *ui.Spinner
	tracker *phaseTrackerWriter
}

// start begins a stage and returns its completion callback.
func (p *buildProgress) start(task build.Task) func(error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	running, complete := buildTaskMessages(task)
	p.spinner = ui.NewSpinner(running)
	p.spinner.Start()
	if task == build.TaskComposer && !p.verbose {
		rules := append(composerPhaseRules(), dockerStartPhaseRules()...)
		p.tracker = newPhaseTrackerWriter(p.spinner, running, rules)
	}

	return func(err error) {
		p.mu.Lock()
		defer p.mu.Unlock()

		switch {
		case p.ctx.Err() != nil:
			p.spinner.Stop()
		case err != nil:
			p.spinner.StopWithMessage("error", running+" failed")
			if p.tracker != nil {
				printHiddenOutputTail("Composer output", p.tracker.TailLines())
			}
		default:
			p.spinner.StopWithMessage("success", complete)
		}
		p.spinner, p.tracker = nil, nil
	}
}

// stop clears any active spinner if the build exits before normal completion.
func (p *buildProgress) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.spinner != nil {
		p.spinner.Stop()
	}
}

// buildProgressWriter captures quiet Composer output and stops the spinner for
// visible output, preserving warnings and configured command streams.
type buildProgressWriter struct {
	progress *buildProgress
	output   io.Writer
}

func (w buildProgressWriter) Write(data []byte) (int, error) {
	p := w.progress
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.tracker != nil {
		return p.tracker.Write(data)
	}
	if p.spinner != nil && p.spinner.Animating() {
		// Output can arrive in partial lines. Keep the spinner stopped for the
		// rest of this stage so its frames cannot erase subprocess output.
		p.spinner.Stop()
	}
	return w.output.Write(data)
}

// buildTaskMessages supplies consistent running and completed stage labels.
func buildTaskMessages(task build.Task) (string, string) {
	switch task {
	case build.TaskComposer:
		return "Installing Composer dependencies", "Composer dependencies installed"
	case build.TaskExport:
		return "Exporting add-on data", "Add-on data exported"
	case build.TaskRollup:
		return "Building JavaScript rollups", "JavaScript rollups built"
	case build.TaskMinify:
		return "Minifying JavaScript", "JavaScript minified"
	case build.TaskExec:
		return "Running build commands", "Build commands completed"
	case build.TaskHashes:
		return "Generating file hashes", "File hashes generated"
	case build.TaskArchive:
		return "Creating release archive", "Release archive created"
	default:
		return "Running build task", "Build task completed"
	}
}

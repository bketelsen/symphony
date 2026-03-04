package config

import (
	"log/slog"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// ConfigWatcher watches a WORKFLOW.md file for changes and reloads config.
type ConfigWatcher struct {
	path     string
	current  *Config
	prompt   string
	mu       sync.RWMutex
	onChange func()
	watcher  *fsnotify.Watcher
	done     chan struct{}
}

// NewConfigWatcher creates a watcher that performs an initial parse and starts
// watching for file changes. The onChange callback is called after each successful reload.
func NewConfigWatcher(path string, onChange func()) (*ConfigWatcher, error) {
	cfg, prompt, err := ParseWorkflowFile(path)
	if err != nil {
		return nil, err
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	cw := &ConfigWatcher{
		path:     path,
		current:  cfg,
		prompt:   prompt,
		onChange: onChange,
		watcher:  w,
		done:     make(chan struct{}),
	}

	if err := w.Add(path); err != nil {
		w.Close()
		return nil, err
	}

	go cw.watch()
	return cw, nil
}

// Current returns the current config and prompt template (thread-safe).
func (cw *ConfigWatcher) Current() (*Config, string) {
	cw.mu.RLock()
	defer cw.mu.RUnlock()
	return cw.current, cw.prompt
}

// Stop shuts down the file watcher.
func (cw *ConfigWatcher) Stop() {
	cw.watcher.Close()
	<-cw.done
}

func (cw *ConfigWatcher) watch() {
	defer close(cw.done)
	for {
		select {
		case event, ok := <-cw.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				cw.reload()
			}
		case err, ok := <-cw.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("config watcher error", "error", err)
		}
	}
}

func (cw *ConfigWatcher) reload() {
	cfg, prompt, err := ParseWorkflowFile(cw.path)
	if err != nil {
		slog.Error("config reload failed, keeping previous config", "error", err)
		return
	}

	cw.mu.Lock()
	cw.current = cfg
	cw.prompt = prompt
	cw.mu.Unlock()

	if cw.onChange != nil {
		cw.onChange()
	}
}

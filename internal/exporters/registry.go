package exporters

import (
	"fmt"
	"sync"
)

// Registry manages all available exporters
type Registry struct {
	mu        sync.RWMutex
	exporters map[string]Exporter // key: exporter name
}

// NewRegistry creates a new exporter registry
func NewRegistry() *Registry {
	return &Registry{
		exporters: make(map[string]Exporter),
	}
}

// Register adds an exporter to the registry
func (r *Registry) Register(e Exporter) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := e.Name()
	if _, exists := r.exporters[name]; exists {
		return fmt.Errorf("exporter already registered: %s", name)
	}

	r.exporters[name] = e
	return nil
}

// Get retrieves an exporter by name
func (r *Registry) Get(name string) (Exporter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, exists := r.exporters[name]
	if !exists {
		return nil, fmt.Errorf("exporter not found: %s", name)
	}

	return e, nil
}

// List returns all registered exporter names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.exporters))
	for name := range r.exporters {
		names = append(names, name)
	}
	return names
}

// GetEnabled returns only exporters that are enabled in config
func (r *Registry) GetEnabled(enabledNames []string) []Exporter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	enabled := make([]Exporter, 0, len(enabledNames))
	for _, name := range enabledNames {
		if e, exists := r.exporters[name]; exists {
			enabled = append(enabled, e)
		}
	}
	return enabled
}

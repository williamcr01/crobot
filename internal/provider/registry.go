package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"crobot/internal/commands"
)

// ModelRegistry stores and filters models from all providers.
type ModelRegistry struct {
	models    []commands.ModelInfo
	mu        sync.RWMutex
	providers []Provider
}

// NewModelRegistry creates an empty model registry.
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models:    make([]commands.ModelInfo, 0),
		providers: make([]Provider, 0),
	}
}

// AddProvider registers a provider whose models should be listed.
func (r *ModelRegistry) AddProvider(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, existing := range r.providers {
		if existing.Name() == p.Name() {
			r.providers[i] = p
			return
		}
	}
	r.providers = append(r.providers, p)
}

// LoadModels fetches models from all registered providers.
func (r *ModelRegistry) LoadModels(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.models = r.models[:0]
	var errs []error
	for _, p := range r.providers {
		if mip, ok := p.(ModelInfoProvider); ok {
			models, err := mip.ListModelInfo(ctx)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", p.Name(), err))
				continue
			}
			for _, m := range models {
				r.models = append(r.models, commands.ModelInfo{
					ID:            m.ID,
					Provider:      p.Name(),
					ContextLength: m.ContextLength,
					Pricing: commands.Pricing{
						InputPerMTok:      m.Pricing.InputPerMTok,
						OutputPerMTok:     m.Pricing.OutputPerMTok,
						CacheReadPerMTok:  m.Pricing.CacheReadPerMTok,
						CacheWritePerMTok: m.Pricing.CacheWritePerMTok,
					},
				})
			}
			continue
		}

		models, err := p.ListModels(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.Name(), err))
			continue
		}
		for _, m := range models {
			r.models = append(r.models, commands.ModelInfo{
				ID:       m,
				Provider: p.Name(),
			})
		}
	}
	return errors.Join(errs...)
}

// GetAll returns all known models.
func (r *ModelRegistry) GetAll() []commands.ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]commands.ModelInfo{}, r.models...)
}

// Filter returns models whose ID or provider contains the prefix (case-insensitive).
func (r *ModelRegistry) Filter(prefix string) []commands.ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if prefix == "" {
		return append([]commands.ModelInfo{}, r.models...)
	}

	var results []commands.ModelInfo
	lowerPrefix := strings.ToLower(prefix)

	for _, m := range r.models {
		if strings.Contains(strings.ToLower(m.ID), lowerPrefix) ||
			strings.Contains(strings.ToLower(m.Provider), lowerPrefix) {
			results = append(results, m)
		}
	}
	return results
}

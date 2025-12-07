package cogito

import (
	"context"
	"errors"
	"sync"

	"github.com/zoobzio/zyn"
)

// Provider defines the interface for LLM providers.
// This matches zyn.Provider interface for compatibility.
type Provider interface {
	Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error)
	Name() string
}

// Context key for provider.
type providerKeyType struct{}

var providerKey = providerKeyType{}

// Global provider fallback.
var (
	globalProvider   Provider
	globalProviderMu sync.RWMutex
)

// ErrNoProvider is returned when no provider can be resolved.
var ErrNoProvider = errors.New("no provider configured: set via context, step-level, or global")

// SetProvider sets the global fallback provider.
// This provider is used when no context or step-level provider is available.
func SetProvider(p Provider) {
	globalProviderMu.Lock()
	defer globalProviderMu.Unlock()
	globalProvider = p
}

// GetProvider returns the global provider, if set.
func GetProvider() Provider {
	globalProviderMu.RLock()
	defer globalProviderMu.RUnlock()
	return globalProvider
}

// WithProvider adds a provider to the context.
// This is the preferred method for provider management.
func WithProvider(ctx context.Context, p Provider) context.Context {
	return context.WithValue(ctx, providerKey, p)
}

// ProviderFromContext retrieves the provider from context, if present.
func ProviderFromContext(ctx context.Context) (Provider, bool) {
	p, ok := ctx.Value(providerKey).(Provider)
	return p, ok
}

// ResolveProvider determines which provider to use based on resolution order:
// 1. Step-level provider (passed as argument)
// 2. Context provider
// 3. Global provider
// 4. Error if none found.
func ResolveProvider(ctx context.Context, stepProvider Provider) (Provider, error) {
	// 1. Step-level provider takes highest priority
	if stepProvider != nil {
		return stepProvider, nil
	}

	// 2. Context provider
	if p, ok := ProviderFromContext(ctx); ok {
		return p, nil
	}

	// 3. Global provider
	globalProviderMu.RLock()
	p := globalProvider
	globalProviderMu.RUnlock()

	if p != nil {
		return p, nil
	}

	// 4. No provider found
	return nil, ErrNoProvider
}

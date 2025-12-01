package cogito

import (
	"context"
	"testing"

	"github.com/zoobzio/zyn"
)

// mockProvider implements Provider for testing
type mockProvider struct {
	name string
}

func (m *mockProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
	return &zyn.ProviderResponse{
		Content: "mock response",
		Usage: zyn.TokenUsage{
			Prompt:     10,
			Completion: 5,
			Total:      15,
		},
	}, nil
}

func (m *mockProvider) Name() string {
	return m.name
}

func TestSetGetProvider(t *testing.T) {
	// Clear global provider first
	SetProvider(nil)

	// Should be nil initially
	if p := GetProvider(); p != nil {
		t.Error("expected nil provider")
	}

	// Set global provider
	mock := &mockProvider{name: "global"}
	SetProvider(mock)

	// Should retrieve it
	p := GetProvider()
	if p == nil {
		t.Fatal("expected provider to be set")
	}

	if p.Name() != "global" {
		t.Errorf("expected name %q, got %q", "global", p.Name())
	}

	// Clean up
	SetProvider(nil)
}

func TestWithProvider(t *testing.T) {
	mock := &mockProvider{name: "context"}
	ctx := WithProvider(context.Background(), mock)

	p, ok := ProviderFromContext(ctx)
	if !ok {
		t.Fatal("expected provider in context")
	}

	if p.Name() != "context" {
		t.Errorf("expected name %q, got %q", "context", p.Name())
	}
}

func TestProviderFromContextMissing(t *testing.T) {
	ctx := context.Background()

	_, ok := ProviderFromContext(ctx)
	if ok {
		t.Error("expected no provider in context")
	}
}

func TestResolveProviderStepLevel(t *testing.T) {
	// Set up all three levels
	SetProvider(&mockProvider{name: "global"})
	ctx := WithProvider(context.Background(), &mockProvider{name: "context"})
	stepProvider := &mockProvider{name: "step"}

	// Step-level should win
	p, err := ResolveProvider(ctx, stepProvider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.Name() != "step" {
		t.Errorf("expected step-level provider, got %q", p.Name())
	}

	// Clean up
	SetProvider(nil)
}

func TestResolveProviderContext(t *testing.T) {
	// Set global but use context
	SetProvider(&mockProvider{name: "global"})
	ctx := WithProvider(context.Background(), &mockProvider{name: "context"})

	// Context should win over global (no step-level)
	p, err := ResolveProvider(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.Name() != "context" {
		t.Errorf("expected context provider, got %q", p.Name())
	}

	// Clean up
	SetProvider(nil)
}

func TestResolveProviderGlobal(t *testing.T) {
	SetProvider(&mockProvider{name: "global"})
	ctx := context.Background()

	// Global should be used (no step-level or context)
	p, err := ResolveProvider(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.Name() != "global" {
		t.Errorf("expected global provider, got %q", p.Name())
	}

	// Clean up
	SetProvider(nil)
}

func TestResolveProviderNone(t *testing.T) {
	// Make sure global is cleared
	SetProvider(nil)
	ctx := context.Background()

	// Should error when no provider is available
	_, err := ResolveProvider(ctx, nil)
	if err == nil {
		t.Error("expected error when no provider is configured")
	}

	if err != ErrNoProvider {
		t.Errorf("expected ErrNoProvider, got %v", err)
	}
}

func TestResolveProviderPriority(t *testing.T) {
	// Test complete priority chain
	global := &mockProvider{name: "global"}
	contextProvider := &mockProvider{name: "context"}
	stepProvider := &mockProvider{name: "step"}

	SetProvider(global)
	defer SetProvider(nil)

	tests := []struct {
		name         string
		ctx          context.Context
		stepProvider Provider
		expected     string
	}{
		{
			name:         "step level wins",
			ctx:          WithProvider(context.Background(), contextProvider),
			stepProvider: stepProvider,
			expected:     "step",
		},
		{
			name:         "context wins over global",
			ctx:          WithProvider(context.Background(), contextProvider),
			stepProvider: nil,
			expected:     "context",
		},
		{
			name:         "global as fallback",
			ctx:          context.Background(),
			stepProvider: nil,
			expected:     "global",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := ResolveProvider(tt.ctx, tt.stepProvider)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if p.Name() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, p.Name())
			}
		})
	}
}

func TestConcurrentProviderAccess(t *testing.T) {
	// Test thread safety of global provider
	mock := &mockProvider{name: "concurrent"}

	// Concurrent sets and gets
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			SetProvider(mock)
			_ = GetProvider()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Should not panic and should have a provider
	p := GetProvider()
	if p == nil {
		t.Error("expected provider after concurrent access")
	}

	// Clean up
	SetProvider(nil)
}

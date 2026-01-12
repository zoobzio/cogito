package cogito

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoobzio/pipz"
	"github.com/zoobzio/zyn"
)

// mockDiscernProvider implements Provider interface for testing Discern.
type mockDiscernProvider struct {
	callCount       int
	primaryResult   string
	secondaryResult string
}

func (m *mockDiscernProvider) Call(ctx context.Context, messages []zyn.Message, temperature float32) (*zyn.ProviderResponse, error) {
	m.callCount++

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	lastMessage := messages[len(messages)-1]

	// Transform synapse call
	if strings.Contains(lastMessage.Content, "Transform:") {
		return &zyn.ProviderResponse{
			Content: fmt.Sprintf(`{"output": "Routed to %s based on classification", "confidence": 0.91, "changes": ["Synthesized routing context"], "reasoning": ["Combined category with original context"]}`, m.primaryResult),
			Usage: zyn.TokenUsage{
				Prompt:     15,
				Completion: 25,
				Total:      40,
			},
		}, nil
	}

	// Classification synapse call
	if strings.Contains(lastMessage.Content, "Task:") && !strings.Contains(lastMessage.Content, "Transform") {
		primary := m.primaryResult
		if primary == "" {
			primary = "billing"
		}
		secondary := m.secondaryResult
		if secondary == "" {
			secondary = "technical"
		}
		return &zyn.ProviderResponse{
			Content: fmt.Sprintf(`{"primary": "%s", "secondary": "%s", "confidence": 0.87, "reasoning": ["Analyzed ticket content", "Determined category"]}`, primary, secondary),
			Usage: zyn.TokenUsage{
				Prompt:     10,
				Completion: 20,
				Total:      30,
			},
		}, nil
	}

	// Default fallback
	return &zyn.ProviderResponse{
		Content: `{"primary": "general", "secondary": "", "confidence": 0.6, "reasoning": ["Unable to determine specific category"]}`,
		Usage: zyn.TokenUsage{
			Prompt:     10,
			Completion: 10,
			Total:      20,
		},
	}, nil
}

func (m *mockDiscernProvider) Name() string {
	return "mock-discern"
}

// mockRouteProcessor is a simple processor that marks which route was taken.
type mockRouteProcessor struct {
	identity pipz.Identity
	name     string
	called   bool
	marker   string
}

func newMockRouteProcessor(name, marker string) *mockRouteProcessor {
	return &mockRouteProcessor{
		identity: pipz.NewIdentity(name, "Mock route processor"),
		name:     name,
		marker:   marker,
	}
}

func (m *mockRouteProcessor) Process(ctx context.Context, t *Thought) (*Thought, error) {
	m.called = true
	t.SetContent(ctx, m.marker, "processed", m.name)
	return t, nil
}

func (m *mockRouteProcessor) Identity() pipz.Identity {
	return m.identity
}

func (m *mockRouteProcessor) Schema() pipz.Node {
	return pipz.Node{Identity: m.identity, Type: "mock-route-processor"}
}

func (m *mockRouteProcessor) Close() error {
	return nil
}

func TestDiscernBasicRouting(t *testing.T) {
	provider := &mockDiscernProvider{primaryResult: "billing"}
	SetProvider(provider)
	defer SetProvider(nil)

	billingRoute := newMockRouteProcessor("billing-handler", "billing_processed")
	technicalRoute := newMockRouteProcessor("technical-handler", "technical_processed")

	router := NewDiscern(
		"ticket_route",
		"What type of support ticket is this?",
		[]string{"billing", "technical", "general"},
	)
	router.AddRoute("billing", billingRoute)
	router.AddRoute("technical", technicalRoute)

	thought := newTestThought("test routing")
	thought.SetContent(context.Background(), "ticket_text", "I have a billing question", "initial")

	result, err := router.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify billing route was taken
	if !billingRoute.called {
		t.Error("expected billing route to be called")
	}
	if technicalRoute.called {
		t.Error("expected technical route to NOT be called")
	}

	// Verify marker note exists
	_, err = result.GetContent("billing_processed")
	if err != nil {
		t.Error("expected billing_processed note to exist")
	}
}

func TestDiscernFallback(t *testing.T) {
	provider := &mockDiscernProvider{primaryResult: "unknown"}
	SetProvider(provider)
	defer SetProvider(nil)

	fallbackRoute := newMockRouteProcessor("fallback-handler", "fallback_processed")

	router := NewDiscern(
		"ticket_route",
		"What type of support ticket is this?",
		[]string{"billing", "technical"},
	)
	router.SetFallback(fallbackRoute)

	thought := newTestThought("test fallback")
	thought.SetContent(context.Background(), "ticket_text", "Something unusual", "initial")

	result, err := router.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify fallback was called
	if !fallbackRoute.called {
		t.Error("expected fallback route to be called")
	}

	// Verify marker note exists
	_, err = result.GetContent("fallback_processed")
	if err != nil {
		t.Error("expected fallback_processed note to exist")
	}
}

func TestDiscernPassThrough(t *testing.T) {
	provider := &mockDiscernProvider{primaryResult: "unknown"}
	SetProvider(provider)
	defer SetProvider(nil)

	// No routes, no fallback - should pass through
	router := NewDiscern(
		"ticket_route",
		"What type of support ticket is this?",
		[]string{"billing", "technical"},
	)

	thought := newTestThought("test pass through")
	thought.SetContent(context.Background(), "ticket_text", "Something unusual", "initial")

	result, err := router.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify categorization note still exists
	resp, err := router.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if resp.Primary != "unknown" {
		t.Errorf("expected primary 'unknown', got %q", resp.Primary)
	}
}

func TestDiscernScan(t *testing.T) {
	provider := &mockDiscernProvider{primaryResult: "technical", secondaryResult: "billing"}
	SetProvider(provider)
	defer SetProvider(nil)

	router := NewDiscern(
		"ticket_route",
		"What type of support ticket is this?",
		[]string{"billing", "technical", "general"},
	)

	thought := newTestThought("test scan")
	thought.SetContent(context.Background(), "ticket_text", "Technical issue", "initial")

	result, err := router.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Use Scan to get typed response
	resp, err := router.Scan(result)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if resp.Primary != "technical" {
		t.Errorf("expected primary 'technical', got %q", resp.Primary)
	}

	if resp.Secondary != "billing" {
		t.Errorf("expected secondary 'billing', got %q", resp.Secondary)
	}

	if resp.Confidence != 0.87 {
		t.Errorf("expected confidence 0.87, got %f", resp.Confidence)
	}
}

func TestDiscernDefaultNoIntrospection(t *testing.T) {
	provider := &mockDiscernProvider{primaryResult: "billing"}
	SetProvider(provider)
	defer SetProvider(nil)

	router := NewDiscern(
		"ticket_route",
		"What type of support ticket is this?",
		[]string{"billing", "technical"},
	)

	thought := newTestThought("test no introspection")
	thought.SetContent(context.Background(), "ticket_text", "Billing question", "initial")

	result, err := router.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify summary note does NOT exist (introspection disabled by default)
	_, err = result.GetContent("ticket_route_summary")
	if err == nil {
		t.Error("expected summary note to not exist by default")
	}

	// Verify only 1 provider call (Classification only)
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.callCount)
	}
}

func TestDiscernWithIntrospection(t *testing.T) {
	provider := &mockDiscernProvider{primaryResult: "billing"}
	SetProvider(provider)
	defer SetProvider(nil)

	router := NewDiscern(
		"ticket_route",
		"What type of support ticket is this?",
		[]string{"billing", "technical"},
	).WithIntrospection()

	thought := newTestThought("test with introspection")
	thought.SetContent(context.Background(), "ticket_text", "Billing question", "initial")

	result, err := router.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify summary note exists (introspection enabled)
	_, err = result.GetContent("ticket_route_summary")
	if err != nil {
		t.Error("expected summary note to exist when introspection enabled")
	}

	// Verify 2 provider calls (Classification + Transform)
	if provider.callCount != 2 {
		t.Errorf("expected 2 provider calls, got %d", provider.callCount)
	}
}

func TestDiscernWithSummaryKey(t *testing.T) {
	provider := &mockDiscernProvider{primaryResult: "billing"}
	SetProvider(provider)
	defer SetProvider(nil)

	router := NewDiscern(
		"ticket_route",
		"What type of support ticket is this?",
		[]string{"billing", "technical"},
	).WithIntrospection().WithSummaryKey("custom_routing_summary")

	thought := newTestThought("test custom summary key")
	thought.SetContent(context.Background(), "ticket_text", "Billing question", "initial")

	result, err := router.Process(context.Background(), thought)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify custom summary key exists
	_, err = result.GetContent("custom_routing_summary")
	if err != nil {
		t.Error("expected custom_routing_summary note to exist")
	}

	// Verify default summary key does NOT exist
	_, err = result.GetContent("ticket_route_summary")
	if err == nil {
		t.Error("expected default summary key to not exist")
	}
}

func TestDiscernRouteManagement(t *testing.T) {
	router := NewDiscern(
		"route",
		"What type?",
		[]string{"a", "b", "c"},
	)

	routeA := newMockRouteProcessor("a-handler", "a_processed")
	routeB := newMockRouteProcessor("b-handler", "b_processed")

	// Add routes
	router.AddRoute("a", routeA)
	router.AddRoute("b", routeB)

	if !router.HasRoute("a") {
		t.Error("expected route 'a' to exist")
	}
	if !router.HasRoute("b") {
		t.Error("expected route 'b' to exist")
	}
	if router.HasRoute("c") {
		t.Error("expected route 'c' to not exist")
	}

	// Check Routes() returns copy
	routes := router.Routes()
	if len(routes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(routes))
	}

	// Remove route
	router.RemoveRoute("a")
	if router.HasRoute("a") {
		t.Error("expected route 'a' to be removed")
	}

	// Clear routes
	router.ClearRoutes()
	if router.HasRoute("b") {
		t.Error("expected all routes to be cleared")
	}
}

func TestDiscernChainable(t *testing.T) {
	router := NewDiscern(
		"ticket_route",
		"What type of support ticket is this?",
		[]string{"billing", "technical"},
	)

	// Verify Identity().Identity().Name() returns expected value
	if router.Identity().Name() != "ticket_route" {
		t.Errorf("expected name 'ticket_route', got %q", router.Identity().Name())
	}

	// Verify Close() doesn't error
	if err := router.Close(); err != nil {
		t.Errorf("unexpected error from Close(): %v", err)
	}
}

// mockFailingProcessor returns an error when processed.
type mockFailingProcessor struct {
	identity pipz.Identity
	name     string
	err      error
}

func newMockFailingProcessor(name string, err error) *mockFailingProcessor {
	return &mockFailingProcessor{
		identity: pipz.NewIdentity(name, "Mock failing processor"),
		name:     name,
		err:      err,
	}
}

func (m *mockFailingProcessor) Process(ctx context.Context, t *Thought) (*Thought, error) {
	return t, m.err
}

func (m *mockFailingProcessor) Identity() pipz.Identity {
	return m.identity
}

func (m *mockFailingProcessor) Schema() pipz.Node {
	return pipz.Node{Identity: m.identity, Type: "mock-failing-processor"}
}

func (m *mockFailingProcessor) Close() error {
	return nil
}

func TestDiscernRouteProcessorError(t *testing.T) {
	provider := &mockDiscernProvider{primaryResult: "billing"}
	SetProvider(provider)
	defer SetProvider(nil)

	routeErr := fmt.Errorf("billing system unavailable")
	failingRoute := newMockFailingProcessor("billing-handler", routeErr)

	router := NewDiscern(
		"ticket_route",
		"What type of support ticket is this?",
		[]string{"billing", "technical"},
	)
	router.AddRoute("billing", failingRoute)

	thought := newTestThought("test route error")
	thought.SetContent(context.Background(), "ticket_text", "Billing question", "initial")

	_, err := router.Process(context.Background(), thought)
	if err == nil {
		t.Fatal("expected error from failing route")
	}

	// Verify error wrapping
	if !strings.Contains(err.Error(), "billing") {
		t.Errorf("expected error to mention route name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "billing system unavailable") {
		t.Errorf("expected error to contain original error, got: %v", err)
	}
}

func TestDiscernFallbackProcessorError(t *testing.T) {
	provider := &mockDiscernProvider{primaryResult: "unknown"}
	SetProvider(provider)
	defer SetProvider(nil)

	fallbackErr := fmt.Errorf("fallback handler failed")
	failingFallback := newMockFailingProcessor("fallback-handler", fallbackErr)

	router := NewDiscern(
		"ticket_route",
		"What type of support ticket is this?",
		[]string{"billing", "technical"},
	)
	router.SetFallback(failingFallback)

	thought := newTestThought("test fallback error")
	thought.SetContent(context.Background(), "ticket_text", "Unknown request", "initial")

	_, err := router.Process(context.Background(), thought)
	if err == nil {
		t.Fatal("expected error from failing fallback")
	}

	// Verify error wrapping
	if !strings.Contains(err.Error(), "fallback") {
		t.Errorf("expected error to mention fallback, got: %v", err)
	}
	if !strings.Contains(err.Error(), "fallback handler failed") {
		t.Errorf("expected error to contain original error, got: %v", err)
	}
}

// mockClosingProcessor tracks Close() calls and can return an error.
type mockClosingProcessor struct {
	identity pipz.Identity
	name     string
	closed   bool
	closeErr error
}

func newMockClosingProcessor(name string, closeErr error) *mockClosingProcessor {
	return &mockClosingProcessor{
		identity: pipz.NewIdentity(name, "Mock closing processor"),
		name:     name,
		closeErr: closeErr,
	}
}

func (m *mockClosingProcessor) Process(ctx context.Context, t *Thought) (*Thought, error) {
	return t, nil
}

func (m *mockClosingProcessor) Identity() pipz.Identity {
	return m.identity
}

func (m *mockClosingProcessor) Schema() pipz.Node {
	return pipz.Node{Identity: m.identity, Type: "mock-closing-processor"}
}

func (m *mockClosingProcessor) Close() error {
	m.closed = true
	return m.closeErr
}

func TestDiscernClosePropagates(t *testing.T) {
	routeA := newMockClosingProcessor("route-a", nil)
	routeB := newMockClosingProcessor("route-b", nil)
	fallback := newMockClosingProcessor("fallback", nil)

	router := NewDiscern(
		"test_route",
		"What type?",
		[]string{"a", "b"},
	)
	router.AddRoute("a", routeA)
	router.AddRoute("b", routeB)
	router.SetFallback(fallback)

	err := router.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !routeA.closed {
		t.Error("expected route A to be closed")
	}
	if !routeB.closed {
		t.Error("expected route B to be closed")
	}
	if !fallback.closed {
		t.Error("expected fallback to be closed")
	}
}

func TestDiscernCloseCollectsErrors(t *testing.T) {
	routeA := newMockClosingProcessor("route-a", fmt.Errorf("route A close failed"))
	routeB := newMockClosingProcessor("route-b", nil)
	fallback := newMockClosingProcessor("fallback", fmt.Errorf("fallback close failed"))

	router := NewDiscern(
		"test_route",
		"What type?",
		[]string{"a", "b"},
	)
	router.AddRoute("a", routeA)
	router.AddRoute("b", routeB)
	router.SetFallback(fallback)

	err := router.Close()
	if err == nil {
		t.Fatal("expected error from Close()")
	}

	// Verify both errors are present (errors.Join separates with newline)
	if !strings.Contains(err.Error(), "route A close failed") {
		t.Errorf("expected error to contain route A error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "fallback close failed") {
		t.Errorf("expected error to contain fallback error, got: %v", err)
	}

	// All should still be closed despite errors
	if !routeA.closed {
		t.Error("expected route A to be closed")
	}
	if !routeB.closed {
		t.Error("expected route B to be closed")
	}
	if !fallback.closed {
		t.Error("expected fallback to be closed")
	}
}

package cogito

import (
	"context"

	"github.com/zoobzio/pipz"
)

// Do creates a processor from a custom function that can fail.
// This is the easiest way to add custom logic to a chain.
//
// Example:
//
//	route := cogito.Do("route-ticket", func(ctx context.Context, t *cogito.Thought) (*cogito.Thought, error) {
//	    ticketType, _ := t.GetContent("ticket_type")
//	    if ticketType == "urgent" {
//	        t.SetContent("queue", "urgent-queue", "route-ticket")
//	    } else {
//	        t.SetContent("queue", "standard-queue", "route-ticket")
//	    }
//	    return t, nil
//	})
func Do(name string, fn func(context.Context, *Thought) (*Thought, error)) pipz.Processor[*Thought] {
	return pipz.Apply(pipz.Name(name), fn)
}

// Transform creates a processor from a pure transformation function.
// Use this when your operation cannot fail.
//
// Example:
//
//	addMetadata := cogito.Transform("add-metadata", func(ctx context.Context, t *cogito.Thought) *cogito.Thought {
//	    t.SetContent("timestamp", time.Now().Format(time.RFC3339), "add-metadata")
//	    return t
//	})
func Transform(name string, fn func(context.Context, *Thought) *Thought) pipz.Processor[*Thought] {
	return pipz.Transform(pipz.Name(name), fn)
}

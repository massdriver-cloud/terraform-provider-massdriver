// Package gqlmock provides genqlient graphql.Client fakes for tests.
package gqlmock

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Khan/genqlient/graphql"
)

// NewClientWithSingleJSONResponse returns a graphql.Client that replies with the
// same JSON-shaped map on every request. The map should contain a top-level
// "data" key (and optionally "errors").
func NewClientWithSingleJSONResponse(response map[string]any) graphql.Client {
	return &Recorder{response: response}
}

// NewClientWithResponses returns a Recorder that dispatches different responses
// by genqlient operation name (e.g., "createProject", "getProject"). A request
// for an operation with no configured response returns an error.
//
// Use this when a single test exercises multiple API calls (e.g., Create
// followed by Read), so each operation can have its own canned response.
func NewClientWithResponses(responses map[string]map[string]any) *Recorder {
	return &Recorder{responsesByOp: responses}
}

// Recorder is a graphql.Client that captures every request it receives so tests
// can assert on the operation name and variables.
type Recorder struct {
	mu            sync.Mutex
	Requests      []*graphql.Request
	response      map[string]any            // single canned response (any operation)
	responsesByOp map[string]map[string]any // canned responses keyed by operation name
}

// MakeRequest implements graphql.Client.
func (r *Recorder) MakeRequest(_ context.Context, req *graphql.Request, resp *graphql.Response) error {
	r.mu.Lock()
	r.Requests = append(r.Requests, req)
	r.mu.Unlock()

	response := r.response
	if r.responsesByOp != nil {
		var ok bool
		response, ok = r.responsesByOp[req.OpName]
		if !ok {
			return fmt.Errorf("gqlmock: no response configured for operation %q", req.OpName)
		}
	}

	if data, ok := response["data"]; ok && resp.Data != nil {
		bytes, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(bytes, resp.Data); err != nil {
			return err
		}
	}
	if errs, ok := response["errors"]; ok {
		envelope, err := json.Marshal(map[string]any{"errors": errs})
		if err != nil {
			return err
		}
		if err := json.Unmarshal(envelope, resp); err != nil {
			return err
		}
		// Mirror the real genqlient client: a non-empty `errors` array surfaces
		// as a Go-level error from MakeRequest, not just as resp.Errors.
		if len(resp.Errors) > 0 {
			return resp.Errors
		}
	}
	return nil
}

// FindRequest returns the first captured request matching the given operation name,
// or nil if no such request was made.
func (r *Recorder) FindRequest(opName string) *graphql.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, req := range r.Requests {
		if req.OpName == opName {
			return req
		}
	}
	return nil
}

// Variables returns the JSON-decoded variables of a captured request, for assertion.
// Returns an empty map if the request is nil or has no variables.
func Variables(req *graphql.Request) map[string]any {
	if req == nil || req.Variables == nil {
		return map[string]any{}
	}
	bytes, err := json.Marshal(req.Variables)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(bytes, &out); err != nil {
		return map[string]any{}
	}
	return out
}

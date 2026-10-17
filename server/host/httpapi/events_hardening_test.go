package httpapi

import (
	"net/http"
	"testing"
)

// TestCreateEventRejectsClientSeed covers the E1-8 API half: a create
// request that carries a seed is a bad request decoded nowhere near the
// backend — the seed is host-minted, never a client field.
func TestCreateEventRejectsClientSeed(t *testing.T) {
	srv, fake := eventsServer(t, Options{})
	st, raw := doReq(t, srv, "POST", "/api/events", "",
		`{"set":"FDN","seed":9,"seats":[{"kind":"bot"},{"kind":"bot"},{"kind":"bot"},{"kind":"bot"}]}`)
	if st != http.StatusBadRequest {
		t.Fatalf("status %d, want 400 (body %s)", st, raw)
	}
	if code := bodyCode(t, raw); code != "bad_request" {
		t.Errorf("code %q, want bad_request", code)
	}
	if fake.lastMethod != "" {
		t.Errorf("backend saw %q; the request must be refused at decode", fake.lastMethod)
	}
}

// TestEventSetsAndPersonasRoutes covers the two discovery routes at the
// HTTP layer: the GETs dispatch to the backend, the POSTs answer 405, and
// with the events tier off both are 404s.
func TestEventSetsAndPersonasRoutes(t *testing.T) {
	srv, fake := eventsServer(t, Options{})
	for _, tc := range []struct {
		name, method, path, wantCode, wantCall string
		wantStatus                             int
	}{
		{"sets", "GET", "/api/events/sets", "", "Sets", 200},
		{"personas", "GET", "/api/events/personas", "", "Personas", 200},
		{"post sets", "POST", "/api/events/sets", "method_not_allowed", "", 405},
		{"post personas", "POST", "/api/events/personas", "method_not_allowed", "", 405},
		{"delete personas", "DELETE", "/api/events/personas", "method_not_allowed", "", 405},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake.lastMethod = ""
			st, raw := doReq(t, srv, tc.method, tc.path, "", "")
			if st != tc.wantStatus {
				t.Fatalf("status %d, want %d (body %s)", st, tc.wantStatus, raw)
			}
			if tc.wantCode != "" && bodyCode(t, raw) != tc.wantCode {
				t.Errorf("code %q, want %q", bodyCode(t, raw), tc.wantCode)
			}
			if fake.lastMethod != tc.wantCall {
				t.Errorf("backend method %q, want %q", fake.lastMethod, tc.wantCall)
			}
		})
	}

	off, _ := eventsServerOff(t)
	for _, path := range []string{"/api/events/sets", "/api/events/personas"} {
		if st, raw := doReq(t, off, "GET", path, "", ""); st != http.StatusNotFound {
			t.Errorf("GET %s with events off: status %d, want 404 (body %s)", path, st, raw)
		}
	}
}

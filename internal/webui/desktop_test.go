package webui

import (
	"strings"
	"testing"
)

func TestDesktopEndpointSecurityAndStrictPlanRequests(t *testing.T) {
	s := fixture(t)
	for _, tc := range []struct {
		body        string
		auth, token bool
		code        int
	}{
		{`{"plans":["x"],"revision":0}`, false, true, 401},
		{`{"plans":["x"],"revision":0}`, true, false, 403},
		{`{"plans":["x"]}`, true, true, 400},
		{`{"plans":["x"],"revision":null}`, true, true, 400},
		{`{"plans":["x"],"revision":0,"command":["arbitrary"]}`, true, true, 400},
		{`{"plans":["x"],"revision":0,"url":"https://evil.example"}`, true, true, 400},
		{`{"plans":[],"revision":0}`, true, true, 400},
		{`{"plans":["x"],"revision":0}`, true, true, 409},
	} {
		response := perform(s, "POST", "/api/desktop/upgrades", tc.body, tc.auth, tc.token)
		if response.Code != tc.code {
			t.Fatalf("%s: got %d (%s), want %d", tc.body, response.Code, response.Body, tc.code)
		}
	}
	response := perform(s, "GET", "/api/state", "", true, false)
	for _, field := range []string{`"desktop":`, `"desktop_tasks":`} {
		if !strings.Contains(response.Body.String(), field) {
			t.Fatalf("missing additive desktop state field %s", field)
		}
	}
}

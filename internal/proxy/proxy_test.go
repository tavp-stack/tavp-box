package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func withTempRoutes(t *testing.T, seed []Route) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "routes.json")
	if seed != nil {
		data, _ := json.MarshalIndent(seed, "", "  ")
		os.WriteFile(path, data, 0644)
	}
	old := routesOverride
	routesOverride = path
	t.Cleanup(func() { routesOverride = old })
}

func readRoutes(t *testing.T, path string) []Route {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read routes: %v", err)
	}
	var r []Route
	json.Unmarshal(data, &r)
	return r
}

// Regression for #24: RemoveRoute on a fresh Proxy instance (empty
// in-memory routes) must not wipe unrelated routes from disk.
func TestRemoveRouteOnFreshInstanceKeepsOtherRoutes(t *testing.T) {
	withTempRoutes(t, []Route{
		{Domain: "a.test", IP: "127.0.0.1", Port: 8001},
		{Domain: "b.test", IP: "127.0.0.1", Port: 8002},
	})

	path := routesOverride
	p := New(80)
	p.RemoveRoute("a.test")

	routes := readRoutes(t, path)
	if len(routes) != 1 || routes[0].Domain != "b.test" {
		t.Fatalf("expected only b.test to remain, got %+v", routes)
	}
}

func TestRemoveRouteMissingDomainKeepsAll(t *testing.T) {
	withTempRoutes(t, []Route{
		{Domain: "a.test", IP: "127.0.0.1", Port: 8001},
		{Domain: "b.test", IP: "127.0.0.1", Port: 8002},
	})

	path := routesOverride
	p := New(80)
	p.RemoveRoute("nope.test")

	routes := readRoutes(t, path)
	if len(routes) != 2 {
		t.Fatalf("expected both routes to remain, got %+v", routes)
	}
}

// AddRoute on a fresh instance must preserve existing disk routes.
func TestAddRouteOnFreshInstancePreservesExisting(t *testing.T) {
	withTempRoutes(t, []Route{
		{Domain: "a.test", IP: "127.0.0.1", Port: 8001},
	})

	path := routesOverride
	p := New(80)
	p.AddRoute("c.test", "127.0.0.1", 8003)

	routes := readRoutes(t, path)
	if len(routes) != 2 {
		t.Fatalf("expected a.test + c.test, got %+v", routes)
	}
}

// Saving an empty route list must produce valid JSON (not "null").
func TestSaveRoutesEmptyProducesValidJSON(t *testing.T) {
	withTempRoutes(t, nil)

	path := routesOverride
	p := New(80)
	p.loadRoutes()
	p.saveRoutes()

	routes := readRoutes(t, path)
	if routes == nil || len(routes) != 0 {
		t.Fatalf("expected empty array, got %+v", routes)
	}
}

// Multi-level subdomains must fall back to the project's base route (#26).
func TestLookupRouteSubdomainFallback(t *testing.T) {
	p := New(80)
	p.routes = []Route{
		{Domain: "penbill.tavp.my.id", IP: "127.0.0.1", Port: 8083},
	}

	cases := map[string]string{
		"penbill.tavp.my.id":     "penbill.tavp.my.id",
		"app.penbill.tavp.my.id": "penbill.tavp.my.id",
		"a.b.penbill.tavp.my.id": "penbill.tavp.my.id",
		"PENBILL.TAVP.MY.ID:443": "penbill.tavp.my.id",
	}
	for host, want := range cases {
		r := p.lookupRoute(host)
		if r == nil || r.Domain != want {
			t.Errorf("lookupRoute(%q) = %v, want %q", host, r, want)
		}
	}

	if r := p.lookupRoute("other.tavp.my.id"); r != nil {
		t.Errorf("unrelated host should not match, got %v", r)
	}
}

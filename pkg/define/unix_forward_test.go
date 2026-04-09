package define

import "testing"

func TestBuildUnixSocketForwardRoutesDeterministic(t *testing.T) {
	t.Parallel()

	routes, err := BuildUnixSocketForwardRoutes(map[string]string{
		"/tmp/b.sock": "/tmp/host-b.sock",
		"/tmp/a.sock": "/tmp/host-a.sock",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}

	if routes[0].GuestPath != "/tmp/a.sock" || routes[0].VSockPort != UnixForwardVSockBasePort {
		t.Fatalf("unexpected first route: %+v", routes[0])
	}
	if routes[1].GuestPath != "/tmp/b.sock" || routes[1].VSockPort != UnixForwardVSockBasePort+1 {
		t.Fatalf("unexpected second route: %+v", routes[1])
	}
}

func TestBuildUnixSocketForwardRoutesEmpty(t *testing.T) {
	t.Parallel()

	routes, err := BuildUnixSocketForwardRoutes(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if routes != nil {
		t.Fatalf("expected nil routes for empty input, got %#v", routes)
	}
}

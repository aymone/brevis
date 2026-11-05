package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"brevis.sh/brevis/internal/api"
	"brevis.sh/brevis/internal/auth"
	"brevis.sh/brevis/internal/domain/catalog"
	"brevis.sh/brevis/internal/infrastructure/endpointcall"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

type endpointsFake struct{ all []postgres.PublishedEndpoint }

func (f endpointsFake) List(context.Context) ([]postgres.PublishedEndpoint, error) { return f.all, nil }
func (f endpointsFake) Get(_ context.Context, api, name string) (postgres.PublishedEndpoint, bool, error) {
	for _, e := range f.all {
		if e.API == api && e.Name == name {
			return e, true, nil
		}
	}
	return postgres.PublishedEndpoint{}, false, nil
}
func (f endpointsFake) Reading(_ context.Context, target string) ([]postgres.PublishedEndpoint, error) {
	var out []postgres.PublishedEndpoint
	for _, e := range f.all {
		for _, r := range e.Reads {
			if r == target {
				out = append(out, e)
			}
		}
	}
	return out, nil
}

func published() endpointsFake {
	def := "last_7_days"
	clicks := catalog.Endpoint{Name: "clicks", Path: "/v1/clicks", Tenant: "key",
		Params: []catalog.EndpointParam{
			{Name: "period", Type: "period", Default: &def},
			{Name: "order", Type: "enum", Values: []string{"asc", "desc"}},
		},
		Limits: catalog.EndpointLimits{Timeout: "10s", MaxRows: 500, MaxBytesBilled: 1 << 30},
		Reads:  []string{"postgres://analytics/marts/clicks_daily"},
		Notes:  map[string]string{"": "Clicks per day, for the dashboard.", "params.order": "Newest first by default."}}
	return endpointsFake{all: []postgres.PublishedEndpoint{{
		API: "metrics-api", Name: "clicks", Path: "/v1/clicks", Tenant: "key",
		Reads: clicks.Reads, Endpoint: clicks, PublishedAt: time.Now().Add(-2 * time.Hour),
	}}}
}

func tryUI(t *testing.T, apiURL string, cred auth.Credential) *api.UI {
	t.Helper()
	calls, err := endpointcall.New("metrics-api="+apiURL, "console-key")
	if err != nil {
		t.Fatal(err)
	}
	return dataUI(catalogFake{}).WithEndpoints(published()).WithEndpointCalls(calls, cred)
}

var loggedIn = auth.Credential{User: "ana", Hash: "$2a$10$notarealhash"}

// ONLY ON A CONSOLE THAT ASKS WHO YOU ARE, and only for an API it has an
// address for. The key is one tenant's for every login: an open console
// would hand that tenant's rows to whoever found the address.
func TestTryItNeedsALoginAndAnAddress(t *testing.T) {
	asked := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { asked = true }))
	defer srv.Close()
	_, body := get(t, tryUI(t, srv.URL, auth.Credential{}), "/data/endpoints/metrics-api/clicks?run=1&period=today")
	if strings.Contains(body, "Try it") || asked {
		t.Error("an open console offered Try it, or called the API")
	}
	other, _ := endpointcall.New("another-api="+srv.URL, "")
	_, body = get(t, dataUI(catalogFake{}).WithEndpoints(published()).WithEndpointCalls(other, loggedIn), "/data/endpoints/metrics-api/clicks")
	if strings.Contains(body, "Try it") {
		t.Error("Try it was offered for an API with no address")
	}
}

// twoAPIs is two endpoints on two APIs, reading different models.
func twoAPIs() endpointsFake {
	f := published()
	req := catalog.Endpoint{Name: "orders", Path: "/v1/orders", Tenant: "key",
		Params: []catalog.EndpointParam{{Name: "day", Type: "date", Required: true}},
		Reads:  []string{"postgres://warehouse/sales/orders", "postgres://warehouse/sales/customers"}}
	f.all = append(f.all, postgres.PublishedEndpoint{API: "sales-api", Name: "orders", Path: "/v1/orders",
		Tenant: "key", Reads: req.Reads, Endpoint: req, PublishedAt: time.Now().Add(-time.Hour)})
	return f
}

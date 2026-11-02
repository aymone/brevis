package iceberg

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	ice "github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/hadoop"
	"github.com/apache/iceberg-go/table"
)

// restServer is an Iceberg REST catalog reduced to what Open and LoadTable
// touch: the /v1/config handshake and one table, bronze.events, with real
// metadata built from the landing schema. It records what the client sent,
// which is what the tests are about.
type restServer struct {
	*httptest.Server
	configs       atomic.Int32
	configQuery   string
	authorization string
	metadataLoc   string
}

func newRESTServer(t *testing.T) *restServer {
	t.Helper()
	sc, err := IcebergSchema(landing)
	if err != nil {
		t.Fatal(err)
	}
	loc := "file:///lake/bronze/events/metadata/00000-abc.metadata.json"
	meta, err := table.NewMetadata(sc, ice.UnpartitionedSpec, table.UnsortedSortOrder, "file:///lake/bronze/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}

	s := &restServer{metadataLoc: loc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/config", func(w http.ResponseWriter, r *http.Request) {
		s.configs.Add(1)
		s.configQuery = r.URL.RawQuery
		s.authorization = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"defaults": map[string]string{}, "overrides": map[string]string{}})
	})
	mux.HandleFunc("GET /v1/namespaces/bronze/tables/events", func(w http.ResponseWriter, r *http.Request) {
		s.authorization = r.Header.Get("Authorization")
		_, _ = fmt.Fprintf(w, `{"metadata-location": %q, "metadata": %s}`, loc, raw)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// unique gives a test its own catalog Name, so the per-process cache never
// hands one test another's catalog, and forgets it afterwards.
func unique(t *testing.T) string {
	t.Helper()
	name := "lake-" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	t.Cleanup(func() { opened.forget(name) })
	return name
}

// ONE NAME IS ONE CATALOG PER PROCESS, the way a Google client is one per
// process: a catalog holds connections and a token, and opening one per
// Write would open a connection per batch. Eight goroutines open the same
// name at once and the server sees one handshake.
func TestTwoOpensOfOneNameShareOneCatalog(t *testing.T) {
	srv := newRESTServer(t)
	c := Catalog{Name: unique(t), Type: REST, URI: srv.URL, Token: "t-1"}

	got := make([]catalog.Catalog, 8)
	var wg sync.WaitGroup
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cat, err := c.Open(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			got[i] = cat
		}()
	}
	wg.Wait()
	for i, cat := range got {
		if cat != got[0] {
			t.Errorf("open %d returned another catalog", i)
		}
	}
	if n := srv.configs.Load(); n != 1 {
		t.Errorf("the server saw %d handshakes, want 1", n)
	}
}

// THE SAME NAME WITH ANOTHER CONFIGURATION IS REFUSED, naming the fields that
// differ and never their values: a refusal that echoes a token is a token in
// a log. Two tables pointing the same Name at two servers is a bug in the
// consumer's wiring, and handing either of them the other's catalog would be
// a write into the wrong lake.
func TestTheSameNameWithAnotherConfigurationIsRefused(t *testing.T) {
	srv := newRESTServer(t)
	name := unique(t)
	if _, err := (Catalog{Name: name, Type: REST, URI: srv.URL, Token: "secret-token"}).Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := (Catalog{Name: name, Type: REST, URI: "https://other.example.com", Token: "other-secret"}).Open(context.Background())
	if err == nil {
		t.Fatal("another URI under the same Name: accepted")
	}
	msg := err.Error()
	for _, want := range []string{fmt.Sprintf("%q", name), "URI", "Token"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not name %s", msg, want)
		}
	}
	for _, secret := range []string{"secret-token", "other-secret", "other.example.com"} {
		if strings.Contains(msg, secret) {
			t.Errorf("error %q echoes %q", msg, secret)
		}
	}
}

// A CATALOG THAT CANNOT BE OPENED IS REFUSED NAMING THE FIELD, before any
// network: a Type this destination does not open, a REST catalog with no
// URI, a SQL catalog with no DSN, a catalog with no Name.
func TestAnUnknownTypeOrAMissingFieldIsRefusedByName(t *testing.T) {
	cases := []struct {
		c    Catalog
		want []string
	}{
		{Catalog{Name: "lake", Type: "glue"}, []string{`"lake"`, "Type", `"glue"`, "rest", "sql"}},
		{Catalog{Name: "lake", Type: REST}, []string{`"lake"`, "URI"}},
		{Catalog{Name: "lake", Type: SQL}, []string{`"lake"`, "DSN"}},
		{Catalog{Type: REST, URI: "http://x"}, []string{"Name"}},
	}
	for _, c := range cases {
		_, err := c.c.Open(context.Background())
		if err == nil {
			t.Errorf("%+v: opened", c.c)
			continue
		}
		for _, want := range c.want {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%+v: error %q does not name %s", c.c, err, want)
			}
		}
	}
}

// THE IO PROPERTIES REACH THE CATALOG UNCHANGED, key by key: they are the
// file IO's own keys -- s3.endpoint, s3.access-key-id, s3.region -- and this
// destination has no opinion about them, so it passes them through rather
// than translating them into a vocabulary of its own that lags behind
// iceberg-go's. The warehouse rides along; nothing else of the Catalog does.
// A DSN or a token is not a file IO property, and the properties go to every
// table the catalog loads.
func TestTheIOPropertiesReachTheCatalogUnchanged(t *testing.T) {
	io := map[string]string{
		"s3.endpoint":                 "http://seaweedfs:8333",
		"s3.access-key-id":            "lake",
		"s3.secret-access-key":        "lake-secret",
		"s3.region":                   "us-east-1",
		"s3.force-virtual-addressing": "false",
	}
	c := Catalog{Name: "lake", Type: SQL, DSN: "postgres://brevis:dsn-secret@db/lake",
		Token: "token-secret", Credential: "id:cred-secret", Warehouse: "s3://lake/warehouse", IO: io}
	p := c.properties()
	for k, v := range io {
		if p[k] != v {
			t.Errorf("%s: %q, want %q", k, p[k], v)
		}
	}
	if p["warehouse"] != "s3://lake/warehouse" {
		t.Errorf("warehouse: %q", p["warehouse"])
	}
	if len(p) != len(io)+1 {
		t.Errorf("%d properties, want the %d IO keys and the warehouse: %v", len(p), len(io)+1, p)
	}
	for k, v := range p {
		for _, secret := range []string{"dsn-secret", "token-secret", "cred-secret"} {
			if strings.Contains(v, secret) {
				t.Errorf("%s carries %s into the file IO", k, secret)
			}
		}
	}
}

// OPEN TALKS TO A REST CATALOG THE WAY THE SPEC SAYS: the handshake carries
// the warehouse, every request carries the token, and the table that comes
// back is the one the server holds -- its metadata location and its schema.
// The server here is an httptest.Server; the real one is D8's.
func TestOpenLoadsATableThroughTheRESTCatalog(t *testing.T) {
	srv := newRESTServer(t)
	c := Catalog{Name: unique(t), Type: REST, URI: srv.URL, Token: "t-9", Warehouse: "s3://lake/warehouse"}
	cat, err := c.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(srv.configQuery, "warehouse=s3") {
		t.Errorf("the handshake did not carry the warehouse: %q", srv.configQuery)
	}
	tbl, err := cat.LoadTable(context.Background(), catalog.ToIdentifier("bronze", "events"))
	if err != nil {
		t.Fatalf("LoadTable: %v", err)
	}
	if tbl.MetadataLocation() != srv.metadataLoc {
		t.Errorf("metadata location %q, want %q", tbl.MetadataLocation(), srv.metadataLoc)
	}
	if got := len(tbl.Schema().Fields()); got != len(landing) {
		t.Errorf("%d fields, want %d", got, len(landing))
	}
	if srv.authorization != "Bearer t-9" {
		t.Errorf("Authorization %q, want the token as a bearer", srv.authorization)
	}
}

// CONN IS RETURNED AS IT IS: a consumer or a test that already holds an
// opened catalog hands it over, and Open neither checks nor caches it. Here
// it is iceberg-go's own filesystem catalog on a temporary directory, which
// is what the Table's tests will use to write without a service.
func TestAnInjectedConnIsReturnedAsItIs(t *testing.T) {
	disk, err := hadoop.NewCatalog("disk", "file://"+t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := (Catalog{Conn: disk}).Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != catalog.Catalog(disk) {
		t.Errorf("Open returned %T, want the injected catalog", got)
	}
}

// THE SQL CATALOG OPENS ON POSTGRES THROUGH pgx, AND A FAILURE NAMES THE
// CATALOG WITHOUT ECHOING THE DSN. The server here is a closed port, which
// refuses at once; the real Postgres is D8's. What this locks in is the
// wrapping: the name, the kind, and no password in the message.
func TestASQLCatalogThatCannotConnectFailsNamingItselfAndNotTheSecret(t *testing.T) {
	c := Catalog{Name: unique(t), Type: SQL, DSN: "postgres://brevis:hunter2@127.0.0.1:1/lake?connect_timeout=1"}
	_, err := c.Open(context.Background())
	if err == nil {
		t.Fatal("a closed port opened")
	}
	msg := err.Error()
	if !strings.Contains(msg, fmt.Sprintf("%q", c.Name)) || !strings.Contains(msg, "sql") {
		t.Errorf("error %q does not name the catalog and its kind", msg)
	}
	if strings.Contains(msg, "hunter2") {
		t.Errorf("error %q echoes the password", msg)
	}
	// A failed open is not cached: the next Open tries again.
	if _, err := c.Open(context.Background()); err == nil {
		t.Error("the second Open against a closed port succeeded")
	}
}

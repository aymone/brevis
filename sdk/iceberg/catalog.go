package iceberg

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"strings"
	"sync"

	ice "github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/rest"
	sqlcat "github.com/apache/iceberg-go/catalog/sql"
	_ "github.com/jackc/pgx/v5/stdlib" // the "pgx" driver database/sql opens the SQL catalog with
)

// CatalogType is the kind of catalog that holds the table's metadata pointer.
type CatalogType string

const (
	// REST is an Iceberg REST catalog: Polaris, Lakekeeper, Nessie, Unity,
	// Gravitino, the reference iceberg-rest image. One HTTP API, many servers.
	REST CatalogType = "rest"

	// SQL is the JDBC-style catalog on Postgres: two tables and a
	// compare-and-swap on the metadata pointer. The smallest catalog a team
	// that already runs Postgres can have.
	SQL CatalogType = "sql"
)

// Catalog is where a table's metadata pointer lives, declared once and
// opened once per process.
//
// The Name is what a Table's Locate and the gateway's YAML call the catalog,
// and the key of the per-process cache: two Tables naming the same catalog
// share one connection and one token, the way the SDK's Google clients are
// one per process. The same Name with another configuration is refused, not
// silently served the first one's catalog.
//
// Credentials never leave the Catalog: Configuration() will list the URI,
// the type and the warehouse, and neither the DSN, the token nor the
// credential. A refusal names fields, not values.
type Catalog struct {
	// Name is the catalog's name in locators and in the cache. Required,
	// unless Conn is set. In a SQL catalog it is also the catalog_name the
	// tables are registered under in Postgres: two processes that share a
	// lake share this Name, and a different Name is a different catalog
	// with no tables in it.
	Name string

	// Type is REST or SQL. Required, unless Conn is set.
	Type CatalogType

	// URI is the REST server: https://polaris.example.com/api/catalog.
	URI string

	// Credential is the REST client credential, "client_id:client_secret",
	// exchanged for a token at the server's OAuth endpoint.
	Credential string

	// Token is a bearer token already issued, for a REST server that wants one.
	Token string

	// Warehouse is where created tables put their files:
	// s3://lake/warehouse, file:///var/lake. A REST server may vend its own.
	Warehouse string

	// DSN is the SQL catalog's Postgres connection string.
	DSN string

	// IO holds the file IO properties, passed to iceberg-go verbatim:
	// s3.endpoint, s3.access-key-id, s3.secret-access-key, s3.region,
	// s3.force-virtual-addressing. They are iceberg-go's keys, not this
	// destination's, so there is no translation to lag behind.
	IO map[string]string

	// Conn is an opened catalog handed over by the consumer or a test. Open
	// returns it as it is, checking and caching nothing.
	Conn catalog.Catalog
}

// Open gives the iceberg-go catalog for this declaration: Conn when set,
// otherwise the process's one catalog for this Name, opened on the first
// call. A failed open is not cached, so the next call tries again.
func (c Catalog) Open(ctx context.Context) (catalog.Catalog, error) {
	if c.Conn != nil {
		return c.Conn, nil
	}
	if err := c.check(); err != nil {
		return nil, err
	}
	return opened.get(c, func() (catalog.Catalog, error) { return c.open(ctx) })
}

// check refuses what cannot be opened, naming the field, before any network.
func (c Catalog) check() error {
	if c.Name == "" {
		return fmt.Errorf("iceberg: a Catalog needs a Name; it is what locators and the per-process cache key on")
	}
	switch c.Type {
	case REST:
		if c.URI == "" {
			return fmt.Errorf("iceberg: catalog %q is %s and has no URI; set URI to the server, as in https://polaris.example.com/api/catalog", c.Name, REST)
		}
	case SQL:
		if c.DSN == "" {
			return fmt.Errorf("iceberg: catalog %q is %s and has no DSN; set DSN to the Postgres connection string", c.Name, SQL)
		}
	default:
		return fmt.Errorf("iceberg: catalog %q has Type %q, which this destination does not open; it opens %q or %q",
			c.Name, c.Type, REST, SQL)
	}
	return nil
}

// open talks to the server. REST through iceberg-go's client, which does the
// /v1/config handshake here; SQL through database/sql on pgx, and the
// catalog's own tables are created on first use, which is iceberg-go's
// default. Neither error echoes a secret: the URI goes into the REST
// message because it is an address, the DSN never does.
func (c Catalog) open(ctx context.Context) (catalog.Catalog, error) {
	switch c.Type {
	case REST:
		opts := []rest.Option{rest.WithAdditionalProps(c.properties())}
		if c.Credential != "" {
			opts = append(opts, rest.WithCredential(c.Credential))
		}
		if c.Token != "" {
			opts = append(opts, rest.WithOAuthToken(c.Token))
		}
		if c.Warehouse != "" {
			opts = append(opts, rest.WithWarehouseLocation(c.Warehouse))
		}
		cat, err := rest.NewCatalog(ctx, c.Name, c.URI, opts...)
		if err != nil {
			return nil, fmt.Errorf("iceberg: opening catalog %q (%s at %s): %w", c.Name, REST, c.URI, err)
		}
		return cat, nil
	case SQL:
		db, err := sql.Open("pgx", c.DSN)
		if err != nil {
			return nil, fmt.Errorf("iceberg: opening catalog %q (%s): %w", c.Name, SQL, err)
		}
		cat, err := sqlcat.NewCatalog(c.Name, db, sqlcat.Postgres, c.properties())
		if err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("iceberg: opening catalog %q (%s): %w", c.Name, SQL, err)
		}
		return cat, nil
	}
	return nil, c.check()
}

// properties is what iceberg-go's catalog receives, and through it every
// table it loads: the IO map verbatim and the warehouse. Nothing else of the
// Catalog -- a DSN, a token, a credential -- is a file IO property.
func (c Catalog) properties() ice.Properties {
	p := make(ice.Properties, len(c.IO)+1)
	maps.Copy(p, c.IO)
	if c.Warehouse != "" {
		p["warehouse"] = c.Warehouse
	}
	return p
}

// opened is the process's catalogs, one per Name.
var opened = catalogs{byName: map[string]*openedCatalog{}}

type catalogs struct {
	mu     sync.Mutex
	byName map[string]*openedCatalog
}

// openedCatalog is one Name's slot: the declaration it was first opened
// with, and the catalog once the open succeeded. Its own lock, so that two
// goroutines opening the same Name at once produce one catalog, and opening
// one Name does not wait on another's handshake.
type openedCatalog struct {
	mu   sync.Mutex
	decl Catalog
	cat  catalog.Catalog
}

func (cs *catalogs) get(c Catalog, open func() (catalog.Catalog, error)) (catalog.Catalog, error) {
	cs.mu.Lock()
	slot, ok := cs.byName[c.Name]
	if !ok {
		slot = &openedCatalog{decl: c}
		cs.byName[c.Name] = slot
	}
	cs.mu.Unlock()

	slot.mu.Lock()
	defer slot.mu.Unlock()
	if fields := differing(slot.decl, c); len(fields) > 0 {
		return nil, fmt.Errorf("iceberg: catalog %q is already declared with another configuration (%s differ); "+
			"one Name is one catalog per process, so give this one another Name", c.Name, strings.Join(fields, ", "))
	}
	if slot.cat != nil {
		return slot.cat, nil
	}
	cat, err := open()
	if err != nil {
		// Not cached, and the slot is given back: the next Open tries again,
		// with this configuration or a corrected one.
		cs.mu.Lock()
		if cs.byName[c.Name] == slot {
			delete(cs.byName, c.Name)
		}
		cs.mu.Unlock()
		return nil, err
	}
	slot.cat = cat
	return cat, nil
}

// forget drops a Name from the cache. For tests, which give each of their
// catalogs a Name of its own and clean up after.
func (cs *catalogs) forget(name string) {
	cs.mu.Lock()
	delete(cs.byName, name)
	cs.mu.Unlock()
}

// differing names the fields two declarations of one Name disagree on,
// without their values: a refusal that echoes a token is a token in a log.
func differing(a, b Catalog) []string {
	var fields []string
	add := func(name string, differs bool) {
		if differs {
			fields = append(fields, name)
		}
	}
	add("Type", a.Type != b.Type)
	add("URI", a.URI != b.URI)
	add("Credential", a.Credential != b.Credential)
	add("Token", a.Token != b.Token)
	add("Warehouse", a.Warehouse != b.Warehouse)
	add("DSN", a.DSN != b.DSN)
	add("IO", !maps.Equal(a.IO, b.IO))
	return fields
}

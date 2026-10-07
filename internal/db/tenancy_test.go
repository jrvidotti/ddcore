package db

import (
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/meta"
)

func tenancyDoc(name string, owned bool) *meta.DocType {
	return &meta.DocType{Name: name, TenantOwned: owned, Fields: []*meta.Field{
		{Fieldname: "code", Fieldtype: "Data", Unique: true},
		{Fieldname: "customer", Fieldtype: "Link", Options: "Customer"},
		{Fieldname: "year", Fieldtype: "Int"},
	}, UniqueKeys: []meta.UniqueKey{{Name: "per_year", Fields: []string{"customer", "year"}}}}
}

// Without tenancy nothing about a table may differ from what it was: the
// column, the key and the indexes are all conditional on TenantOwned.
func TestTenancyOffLeavesTheTableAlone(t *testing.T) {
	d := tenancyDoc("Thing", false)
	ddl := createTable(d)
	if strings.Contains(ddl, "tenant") {
		t.Fatalf("a table without tenancy mentions tenant:\n%s", ddl)
	}
	if !strings.Contains(ddl, `"id" text NOT NULL PRIMARY KEY`) {
		t.Fatalf("the id is no longer the primary key:\n%s", ddl)
	}
	for name, i := range wantedIndexes(d) {
		if strings.Contains(i.cols, "tenant") {
			t.Errorf("%s: %s", name, i.cols)
		}
	}
	if st := planTenancy(&catalog{}, d, true); len(st) != 0 {
		t.Fatalf("planned %v", st)
	}
}

func TestTenantOwnedTableIsKeyedAndIndexedByTenant(t *testing.T) {
	d := tenancyDoc("Thing", true)
	ddl := createTable(d)
	for _, want := range []string{
		`"tenant" text NOT NULL DEFAULT coalesce(current_setting('ddcore.tenant', true), '')`,
		`PRIMARY KEY (tenant, id)`,
		`"id" text NOT NULL,`,
	} {
		if !strings.Contains(ddl, want) {
			t.Errorf("missing %q in:\n%s", want, ddl)
		}
	}
	idx := wantedIndexes(d)
	want := map[string]string{
		"tab_thing_modified":    "tenant, modified DESC",
		"tab_thing_code":        `tenant, "code"`,
		"tab_thing_customer":    `tenant, "customer"`,
		"tab_thing_uk_per_year": `tenant, "customer", "year"`,
	}
	for name, cols := range want {
		if idx[name].cols != cols {
			t.Errorf("%s: cols %q, want %q", name, idx[name].cols, cols)
		}
	}
}

// Sign-in finds a user by e-mail before it knows a tenant.
func TestUserKeepsASiteWideKey(t *testing.T) {
	d := &meta.DocType{Name: "User", TenantOwned: true, Fields: []*meta.Field{
		{Fieldname: "email", Fieldtype: "Data", Unique: true},
		{Fieldname: "manager", Fieldtype: "Link", Options: "User"},
	}}
	ddl := createTable(d)
	if !strings.Contains(ddl, `"id" text NOT NULL PRIMARY KEY`) || strings.Contains(ddl, "PRIMARY KEY (tenant, id)") {
		t.Fatalf("User lost its site-wide key:\n%s", ddl)
	}
	idx := wantedIndexes(d)
	if got := idx["tab_user_email"].cols; got != `"email"` {
		t.Errorf("unique e-mail index is %q", got)
	}
	if got := idx["tab_user_manager"].cols; got != `tenant, "manager"` {
		t.Errorf("plain index is %q", got)
	}
	cat := &catalog{pk: map[string]pkRow{"tab_user": {name: "tab_user_pkey", def: "PRIMARY KEY (id)"}},
		secured: map[string]bool{}, policy: map[string]bool{}}
	for _, st := range planTenancy(cat, d, true) {
		if st.Kind == KindPrimaryKey {
			t.Fatalf("planned a key change for User: %s", st.SQL)
		}
	}
}

func TestPlanTenancyIsWhatTheTableStillLacks(t *testing.T) {
	d := tenancyDoc("Thing", true)
	cat := &catalog{pk: map[string]pkRow{"tab_thing": {name: "tab_thing_pkey", def: "PRIMARY KEY (id)"}},
		secured: map[string]bool{}, policy: map[string]bool{}}
	got := SQL(planTenancy(cat, d, true))
	want := []string{
		`ALTER TABLE "tab_thing" DROP CONSTRAINT "tab_thing_pkey", ADD PRIMARY KEY (tenant, id);`,
		`ALTER TABLE "tab_thing" ENABLE ROW LEVEL SECURITY;`,
		`CREATE POLICY ddcore_tenant ON "tab_thing" USING (tenant = coalesce(current_setting('ddcore.tenant', true), '')) WITH CHECK (tenant = coalesce(current_setting('ddcore.tenant', true), ''));`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
	cat = &catalog{pk: map[string]pkRow{"tab_thing": {name: "tab_thing_pkey", def: "PRIMARY KEY (tenant, id)"}},
		secured: map[string]bool{"tab_thing": true}, policy: map[string]bool{"tab_thing": true}}
	if st := planTenancy(cat, d, true); len(st) != 0 {
		t.Fatalf("a finished table still plans %v", SQL(st))
	}
}

// A table keeps its row-level security, its policy and its key across ALTER
// TABLE … RENAME, so planning them again for the new name fails the migration
// on a policy that already exists (#64).
func TestRenameCarriesTheTenantPolicy(t *testing.T) {
	d := tenancyDoc("Pix Cob", true)
	d.RenamedFrom = meta.Names{"Payment"}
	reg := meta.NewRegistry()
	reg.DocTypes[d.Name] = d
	cat := &catalog{singles: map[string]bool{},
		cols:    map[string]map[string]string{"tab_payment": {"id": "text", "tenant": "text"}},
		idx:     map[string]idxRow{"tab_payment_pkey": {table: "tab_payment", def: "CREATE UNIQUE INDEX tab_payment_pkey ON tab_payment (tenant, id)"}},
		pk:      map[string]pkRow{"tab_payment": {name: "tab_payment_pkey", def: "PRIMARY KEY (tenant, id)"}},
		secured: map[string]bool{"tab_payment": true}, policy: map[string]bool{"tab_payment": true}}
	tables, _, _, _, _ := planRenames(cat, reg, []string{d.Name})
	if len(tables) != 2 {
		t.Fatalf("expected the table and its key renamed, got:\n%s", strings.Join(SQL(tables), "\n"))
	}
	if st := planTenancy(cat, d, true); len(st) != 0 {
		t.Fatalf("the renamed table plans its tenancy again:\n%s", strings.Join(SQL(st), "\n"))
	}
	if got := cat.pk["tab_pix_cob"].name; got != "tab_pix_cob_pkey" {
		t.Fatalf("the key's constraint is %q", got)
	}
}

// A DocType made shared after tenancy was applied sheds what tenancy gave its
// table, in an order Postgres accepts: the policy names the column, so it goes
// first, and the column goes last (#109).
func TestPlanSharingIsTheReverse(t *testing.T) {
	d := tenancyDoc("Thing", false)
	cat := &catalog{cols: map[string]map[string]string{"tab_thing": {"id": "text", "tenant": "text"}},
		pk:      map[string]pkRow{"tab_thing": {name: "tab_thing_pkey", def: "PRIMARY KEY (tenant, id)"}},
		secured: map[string]bool{"tab_thing": true}, policy: map[string]bool{"tab_thing": true}}
	alter, last := planSharing(cat, d)
	want := []string{
		`DROP POLICY ddcore_tenant ON "tab_thing";`,
		`ALTER TABLE "tab_thing" DISABLE ROW LEVEL SECURITY;`,
		`ALTER TABLE "tab_thing" DROP CONSTRAINT "tab_thing_pkey", ADD PRIMARY KEY (id);`,
	}
	if got := SQL(alter); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("alter:\n%s", strings.Join(got, "\n"))
	}
	if len(last) != 1 || last[0].SQL != `ALTER TABLE "tab_thing" DROP COLUMN "tenant";` || last[0].Destructive {
		t.Fatalf("last: %+v", last)
	}
	// a table born shared has nothing to shed
	cat = &catalog{cols: map[string]map[string]string{"tab_thing": {"id": "text"}},
		pk:      map[string]pkRow{"tab_thing": {name: "tab_thing_pkey", def: "PRIMARY KEY (id)"}},
		secured: map[string]bool{}, policy: map[string]bool{}}
	if alter, last := planSharing(cat, d); len(alter)+len(last) != 0 {
		t.Fatalf("a shared table plans %v %v", SQL(alter), SQL(last))
	}
	// and a tenant-owned one is planTenancy's
	if alter, last := planSharing(cat, tenancyDoc("Thing", true)); len(alter)+len(last) != 0 {
		t.Fatalf("a tenant-owned table plans %v %v", SQL(alter), SQL(last))
	}
}

func TestValidTenantID(t *testing.T) {
	for _, ok := range []string{"a", "acme", "acme-2", "a_b", "0x"} {
		if !ValidTenantID(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "Acme", "a'b", "a b", "-a", "a;drop", strings.Repeat("a", 64)} {
		if ValidTenantID(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

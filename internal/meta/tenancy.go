package meta

// TenantColumn is the standard column a tenant-owned table carries. The
// engine never writes it: the column's default stamps every INSERT with the
// space of the transaction, and no UPDATE names it.
const TenantColumn = "tenant"

// TenantDocType is the core DocType whose documents are the tenants. It is
// not called "Tenant": that is the name an app about leases gives its own
// DocType. It is loaded only on a site with tenancy.
const TenantDocType = "Site Tenant"

// TenantTable is its table.
const TenantTable = "tab_site_tenant"

// ApplyTenancy decides, for a site with tenancy on, which DocTypes belong to
// a tenant: every one that does not declare `shared`, a virtual DocType aside
// (it has no table; each of its sources is confined on its own). A child
// table follows the DocTypes that use it, so `shared` on a child is never
// needed — validateTenancy refuses one used from both sides.
//
// With tenancy off it marks nothing, and a site is exactly what it was.
func (r *Registry) ApplyTenancy(enabled bool) {
	r.Tenancy = enabled
	for _, d := range r.DocTypes {
		d.TenantOwned = enabled && !d.Shared && d.Virtual == nil && !d.IsChild
	}
	if !enabled {
		return
	}
	for _, d := range r.DocTypes {
		if !d.IsChild {
			continue
		}
		parents := r.parentsOf(d.Name)
		d.TenantOwned = !d.Shared
		for _, p := range parents {
			if p.TenantOwned {
				d.TenantOwned = true
				break
			}
			d.TenantOwned = false
		}
	}
}

// The values of `space` on a DocType, and on an app for its DocTypes.
const (
	// SpaceTenant: the DocType lives inside a tenant, and the platform space
	// neither reads nor writes it.
	SpaceTenant = "tenant"
	// SpaceAny: the DocType is reached from every space, whatever the app's
	// default says. Only a DocType declares it.
	SpaceAny = "any"
)

// TenantAccessServer is the `tenantAccess` of a shared DocType that, inside a
// tenant, only server code reaches.
const TenantAccessServer = "server"

// ServerOnlyInTenant reports whether, inside a tenant, only server code that
// ignores permissions reaches the DocType: one platform table every tenant's
// code reads and adds to, and no tenant's user may browse (#108).
func (d *DocType) ServerOnlyInTenant() bool {
	return d.Shared && d.TenantAccess == TenantAccessServer
}

// ApplySpaces decides, after ApplyTenancy, which tenant-owned DocTypes are
// the tenants' alone: those that say `space: "tenant"`, and those that say
// nothing in an app whose default (appSpace, by app name) is "tenant". A
// shared or virtual DocType, and a child table, is never tenant-only — the
// parent's own check covers its rows.
func (r *Registry) ApplySpaces(appSpace map[string]string) {
	for _, d := range r.DocTypes {
		space := d.Space
		if space == "" {
			space = appSpace[d.App]
		}
		d.TenantOnly = d.TenantOwned && !d.IsChild && space == SpaceTenant
	}
}

// validateSpace holds `space` to its values, with tenancy on or off, so a typo
// fails the load instead of leaving a DocType reachable.
func validateSpace(d *DocType, e func(string, ...any)) {
	switch {
	case d.TenantAccess == "":
	case d.TenantAccess != TenantAccessServer:
		e("tenantAccess %q is not %q", d.TenantAccess, TenantAccessServer)
	case !d.Shared:
		e("tenantAccess is for a shared DocType: a tenant's own DocType is already its own")
	}
	switch {
	case d.Space == "":
	case d.Space != SpaceTenant && d.Space != SpaceAny:
		e("space %q is neither %q nor %q", d.Space, SpaceTenant, SpaceAny)
	case d.IsChild:
		e("a child DocType has no space of its own: it follows the DocTypes that use it")
	case d.Virtual != nil:
		e("a virtual DocType has no space of its own: each of its sources is confined on its own")
	case d.Shared && d.Space == SpaceTenant:
		e("a shared DocType is read in every space; it cannot also be space %q", SpaceTenant)
	}
}

// parentsOf lists the non-child DocTypes holding a table of child rows.
func (r *Registry) parentsOf(child string) []*DocType {
	var out []*DocType
	for _, name := range r.Names() {
		d := r.DocTypes[name]
		if d.IsChild {
			continue
		}
		for _, f := range d.TableFields() {
			if f.OptionsString() == child {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

// validateTenancy holds the two rules that keep a shared DocType shared: its
// rows are the same for every tenant, so they cannot point at a document only
// one tenant has, nor hold child rows that some other DocType stores per
// tenant.
func (r *Registry) validateTenancy(d *DocType, e func(string, ...any)) {
	validateSpace(d, e)
	if !r.Tenancy {
		return
	}
	if d.IsChild {
		owned, shared := false, false
		for _, p := range r.parentsOf(d.Name) {
			if p.TenantOwned {
				owned = true
			} else {
				shared = true
			}
		}
		if owned && shared {
			e("is a child table of both a shared and a tenant-owned DocType; use one child DocType for each")
		}
		return
	}
	if d.TenantOwned || d.Virtual != nil {
		return
	}
	for _, f := range d.Fields {
		if f.Fieldtype != "Link" {
			continue
		}
		if t, ok := r.Get(f.OptionsString()); ok && t.TenantOwned {
			e("field %q: a shared DocType cannot link to %q, which belongs to a tenant", f.Fieldname, t.Name)
		}
	}
}

// TenantKeyed reports whether the table's rows are addressed by (tenant, id)
// rather than by id alone. User is tenant-owned but keeps a site-wide id:
// sign-in looks a user up by e-mail before it knows any tenant, and sessions,
// API keys and tokens all name a user and nothing else.
func (d *DocType) TenantKeyed() bool { return d.TenantOwned && d.Name != "User" }

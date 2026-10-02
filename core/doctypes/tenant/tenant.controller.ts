import { defineController, _ } from "@ddcore/sdk";

export default defineController("Tenant", {
  validate(doc) {
    // the id is written into SQL that takes no parameters: the shape is the guard
    if (!/^[a-z0-9][a-z0-9_-]{0,62}$/.test(String(doc.slug ?? ""))) {
      ddcore.throw(_("A tenant's slug is lowercase letters, digits, hyphens and underscores, and starts with a letter or digit"),
        { title: _("Invalid slug") });
    }
  },
  // every app seeds the new tenant, however it was created
  afterInsert(doc) { (ddcore as any).tenant.__created(doc.id); },
  onUpdate(doc) { ddcore.cache.del("tenant_state:" + doc.id); },
  afterDelete(doc) { ddcore.cache.del("tenant_state:" + doc.id); },
});

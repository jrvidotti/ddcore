import { defineController } from "@ddcore/sdk";

export default defineController("Letter Head", {
  // One default: saving this one as the default clears the flag on the others.
  // setValue runs no hooks, so this does not recurse.
  onUpdate(doc) {
    if (!doc.is_default) return;
    const others = ddcore.db.getAll("Letter Head", {
      fields: ["name"],
      filters: { is_default: true, name: ["!=", doc.name] },
    });
    for (const other of others) {
      ddcore.db.setValue("Letter Head", other.name, "is_default", false);
    }
  },
});

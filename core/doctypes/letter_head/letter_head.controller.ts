import { defineController } from "@ddcore/sdk";

export default defineController("Letter Head", {
  // One default: saving this one as the default clears the flag on the others.
  // A disabled Letter Head never becomes the effective default (see print.ts),
  // so saving it as default must not clear a working one either, or a print
  // would silently lose its letterhead. setValue runs no hooks, so this does
  // not recurse.
  onUpdate(doc) {
    if (!doc.is_default || doc.disabled) return;
    const others = ddcore.db.getAll("Letter Head", {
      fields: ["name"],
      filters: { is_default: true, name: ["!=", doc.name] },
    });
    for (const other of others) {
      ddcore.db.setValue("Letter Head", other.name, "is_default", false);
    }
  },
});

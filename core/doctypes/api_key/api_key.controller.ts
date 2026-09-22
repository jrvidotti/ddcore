import { defineController } from "@ddcore/sdk";

export default defineController("API Key", {
  onUpdate(doc) { ddcore.cache.del("apikey:" + doc.id); },
  afterDelete(doc) { ddcore.cache.del("apikey:" + doc.id); },
});

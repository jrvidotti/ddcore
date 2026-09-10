import { defineController } from "@cerne/sdk";

export default defineController("API Key", {
  onUpdate(doc) { cerne.cache.del("apikey:" + doc.name); },
  afterDelete(doc) { cerne.cache.del("apikey:" + doc.name); },
});

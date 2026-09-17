import { api, type DocShare, type ShareArgs } from "./api";

/** The sharing panel of one form (SEC-03). */
export class DocSharesState {
  rows = $state<DocShare[]>([]);
  canShare = $state(false);
  canOverrideScope = $state(false);
  loading = $state(false);
  error = $state("");
  pending = $state("");

  constructor(public doctype: string, public name: string) {}

  async load() {
    this.loading = true;
    this.error = "";
    try {
      const res = await api.shares.forDoc(this.doctype, this.name);
      this.rows = res.shares;
      this.canShare = res.canShare;
      this.canOverrideScope = res.canOverrideScope;
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
      this.rows = [];
      this.canShare = false;
      this.canOverrideScope = false;
    } finally {
      this.loading = false;
    }
  }

  async add(args: ShareArgs) {
    await api.shares.add(this.doctype, this.name, args);
    await this.load();
  }

  async remove(user: string) {
    this.pending = user;
    try {
      await api.shares.remove(this.doctype, this.name, user);
      this.rows = this.rows.filter((r) => r.user !== user);
    } finally {
      this.pending = "";
    }
  }
}

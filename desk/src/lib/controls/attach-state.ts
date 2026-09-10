export type AttachAction = "attach" | "replace" | "remove";

export function attachAction(value: unknown, mandatory: boolean): AttachAction {
  if (!value) return "attach";
  return mandatory ? "replace" : "remove";
}

export async function runAttachOperation<T>(setBusy: (busy: boolean) => void, operation: () => Promise<T>): Promise<T> {
  setBusy(true);
  try { return await operation(); } finally { setBusy(false); }
}

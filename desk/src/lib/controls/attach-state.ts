export type AttachAction = "attach" | "replace" | "remove";

export function attachAction(value: unknown, mandatory: boolean): AttachAction {
  if (!value) return "attach";
  return mandatory ? "replace" : "remove";
}

export async function runAttachOperation<T>(setBusy: (busy: boolean) => void, operation: () => Promise<T>): Promise<T> {
  setBusy(true);
  try { return await operation(); } finally { setBusy(false); }
}

/** Last path segment of a file url: the stored (random) name. */
export function storedName(url: unknown): string {
  return String(url ?? "").split("/").pop() || "";
}

/** The name to show for a file: the original one when known, else the stored one. */
export function fileLabel(info: { file_name?: string | null } | null | undefined, url: unknown): string {
  return info?.file_name || storedName(url);
}

/** Bytes as a short human size (1 decimal above bytes, 1024 steps). */
export function formatFileSize(bytes: unknown): string {
  const n = Number(bytes);
  if (bytes === null || bytes === undefined || bytes === "" || !Number.isFinite(n) || n < 0) return "";
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = n / 1024, i = 0;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(1)} ${units[i]}`;
}

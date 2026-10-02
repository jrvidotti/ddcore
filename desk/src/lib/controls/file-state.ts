/** What a dialog's File field hands to the script: the chosen file, never uploaded. */
export interface DialogFile {
  name: string;
  size: number;
  type: string;
  base64: string;
}

/** The cap on a File field that declares no `maxBytes`. */
export const DEFAULT_FILE_MAX_BYTES = 5 * 1024 * 1024;

/** The cap a field's `maxBytes` asks for; anything that is not a positive number is the default. */
export function fileMaxBytes(maxBytes: unknown): number {
  const n = Number(maxBytes);
  return Number.isFinite(n) && n > 0 ? n : DEFAULT_FILE_MAX_BYTES;
}

export function fileTooLarge(size: number, maxBytes: unknown): boolean {
  return size > fileMaxBytes(maxBytes);
}

export function bytesToBase64(bytes: Uint8Array): string {
  // in chunks: String.fromCharCode takes its bytes as arguments, and a large
  // file spread at once overflows the stack
  const chunk = 0x8000;
  let binary = "";
  for (let i = 0; i < bytes.length; i += chunk) binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
  return btoa(binary);
}

export async function readDialogFile(file: File): Promise<DialogFile> {
  const bytes = new Uint8Array(await file.arrayBuffer());
  return { name: file.name, size: file.size, type: file.type, base64: bytesToBase64(bytes) };
}

/**
 * A Barcode field stores text and draws it in its symbology — `options`:
 * Code128 (the default), EAN-13 or QR. The server draws it (`/api/barcode`),
 * so the preview and the printed label come from the same encoder, and the
 * camera reads it with the browser's BarcodeDetector where there is one.
 */
import type { Field } from "$lib/meta";

export type Symbology = "Code128" | "EAN-13" | "QR";

/** The symbology a field draws: its `options`, or Code128. */
export function symbologyOf(field: Pick<Field, "options">): string {
  const o = typeof field.options === "string" ? field.options.trim() : "";
  return o || "Code128";
}

/** The image of `value` in `symbology`, or "" when there is nothing to draw. */
export function barcodeUrl(symbology: string, value: unknown): string {
  const v = String(value ?? "").trim();
  if (!v) return "";
  return "/api/barcode?" + new URLSearchParams({ symbology: symbology || "Code128", value: v }).toString();
}

/** BarcodeDetector's name for each symbology. */
export const DETECTOR_FORMATS: Record<string, string> = {
  Code128: "code_128",
  "EAN-13": "ean_13",
  QR: "qr_code",
};

/** The format to ask BarcodeDetector for, Code128's when the symbology is unknown. */
export function detectorFormat(symbology: string): string {
  return DETECTOR_FORMATS[symbology] ?? DETECTOR_FORMATS.Code128;
}

/**
 * Whether the camera can read a barcode here: the browser has
 * BarcodeDetector (Chromium, Android) and the page is a secure context, which
 * getUserMedia requires.
 */
export function scanAvailable(win: any = typeof window === "undefined" ? undefined : window): boolean {
  return !!win && "BarcodeDetector" in win && !!win.isSecureContext && !!win.navigator?.mediaDevices?.getUserMedia;
}

/** The value a text box commits: trimmed, and a blank is no value. */
export function committedBarcode(text: unknown): string | null {
  const s = String(text ?? "").trim();
  return s === "" ? null : s;
}

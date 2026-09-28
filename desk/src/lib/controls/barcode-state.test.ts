import { describe, expect, it } from "vitest";
import { barcodeUrl, committedBarcode, detectorFormat, scanAvailable, symbologyOf } from "./barcode-state";

describe("barcode-state", () => {
  it("reads the symbology from options, Code128 by default", () => {
    expect(symbologyOf({})).toBe("Code128");
    expect(symbologyOf({ options: " QR " })).toBe("QR");
    expect(symbologyOf({ options: ["QR"] })).toBe("Code128");
  });

  it("builds the endpoint URL, encoding the value", () => {
    expect(barcodeUrl("EAN-13", "400638133393")).toBe("/api/barcode?symbology=EAN-13&value=400638133393");
    expect(barcodeUrl("QR", "https://x.dev/?a=1&b=2")).toBe("/api/barcode?symbology=QR&value=https%3A%2F%2Fx.dev%2F%3Fa%3D1%26b%3D2");
    expect(barcodeUrl("", " A ")).toBe("/api/barcode?symbology=Code128&value=A");
    expect(barcodeUrl("QR", "  ")).toBe("");
    expect(barcodeUrl("QR", null)).toBe("");
  });

  it("maps each symbology to its BarcodeDetector format", () => {
    expect(detectorFormat("Code128")).toBe("code_128");
    expect(detectorFormat("EAN-13")).toBe("ean_13");
    expect(detectorFormat("QR")).toBe("qr_code");
    expect(detectorFormat("nope")).toBe("code_128");
  });

  it("offers the camera only with BarcodeDetector in a secure context", () => {
    const media = { mediaDevices: { getUserMedia: () => {} } };
    expect(scanAvailable({ BarcodeDetector: class {}, isSecureContext: true, navigator: media })).toBe(true);
    expect(scanAvailable({ BarcodeDetector: class {}, isSecureContext: false, navigator: media })).toBe(false);
    expect(scanAvailable({ isSecureContext: true, navigator: media })).toBe(false);
    expect(scanAvailable({ BarcodeDetector: class {}, isSecureContext: true, navigator: {} })).toBe(false);
    expect(scanAvailable(undefined)).toBe(false);
  });

  it("commits trimmed text, and a blank as null", () => {
    expect(committedBarcode(" 123 ")).toBe("123");
    expect(committedBarcode("  ")).toBe(null);
  });
});

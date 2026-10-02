import { describe, expect, it } from "vitest";
import { bytesToBase64, DEFAULT_FILE_MAX_BYTES, fileMaxBytes, fileTooLarge, readDialogFile } from "./file-state";

describe("file-state", () => {
  it("encodes bytes as base64, binary ones included", () => {
    expect(bytesToBase64(new Uint8Array([]))).toBe("");
    expect(bytesToBase64(new TextEncoder().encode("hello"))).toBe("aGVsbG8=");
    expect(bytesToBase64(new Uint8Array([0, 255, 128, 10]))).toBe("AP+ACg==");
  });

  it("encodes more than one chunk", () => {
    const bytes = new Uint8Array(100_000).map((_, i) => i % 251);
    expect(bytesToBase64(bytes)).toBe(Buffer.from(bytes).toString("base64"));
  });

  it("reads a file into what the script receives", async () => {
    const file = new File([new Uint8Array([0, 255, 128, 10])], "cert.pfx", { type: "application/x-pkcs12" });
    expect(await readDialogFile(file)).toEqual({ name: "cert.pfx", size: 4, type: "application/x-pkcs12", base64: "AP+ACg==" });
  });

  it("caps the size at maxBytes, or at the default", () => {
    expect(fileMaxBytes(undefined)).toBe(DEFAULT_FILE_MAX_BYTES);
    expect(fileMaxBytes(0)).toBe(DEFAULT_FILE_MAX_BYTES);
    expect(fileMaxBytes("nope")).toBe(DEFAULT_FILE_MAX_BYTES);
    expect(fileMaxBytes(2048)).toBe(2048);
    expect(fileTooLarge(2048, 2048)).toBe(false);
    expect(fileTooLarge(2049, 2048)).toBe(true);
    expect(fileTooLarge(DEFAULT_FILE_MAX_BYTES + 1, undefined)).toBe(true);
  });
});

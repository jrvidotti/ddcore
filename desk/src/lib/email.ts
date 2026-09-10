import { __ } from "./boot.svelte";
import type { Field } from "./meta";

export const normalizeEmail = (value: unknown): string => String(value ?? "").trim();

const asciiAlphaNumeric = (value: string): boolean => /^[A-Za-z0-9]+$/.test(value);
const localPart = /^[A-Za-z0-9.!#$%&'*+/=?^_`{|}~-]+$/;

export function validEmail(value: string): boolean {
  if (value === "") return true;
  if (value.length > 254 || value.split("@").length !== 2) return false;
  const [local, domain] = value.split("@");
  if (!local || local.length > 64 || !domain || domain.length > 253 || !domain.includes(".")) return false;
  if (local.startsWith(".") || local.endsWith(".") || local.includes("..") || !localPart.test(local)) return false;
  return domain.split(".").every((label) =>
    label.length > 0 && label.length <= 63 && !label.startsWith("-") && !label.endsWith("-") &&
    [...label].every((character) => character === "-" || asciiAlphaNumeric(character)),
  );
}

export function isSystemUserEmail(doctype: string, fieldname: string, value: string): boolean {
  return doctype === "User" && fieldname === "email" && (value === "Administrator" || value === "Guest");
}

export function validateEmailFields(fields: Field[], doc: Record<string, any>, doctype: string): Record<string, string> {
  const errors: Record<string, string> = {};
  for (const field of fields) {
    if (field.fieldtype !== "Email" || !field.fieldname) continue;
    const value = normalizeEmail(doc[field.fieldname]);
    doc[field.fieldname] = value || null;
    if (!validEmail(value) && !isSystemUserEmail(doctype, field.fieldname, value)) errors[field.fieldname] = __("Invalid email");
  }
  return errors;
}

import assert from "node:assert/strict";
import test from "node:test";

import { normalizeEmail, validateEmailFields, validEmail } from "./email.ts";

test("email normalization removes surrounding whitespace", () => {
  assert.equal(normalizeEmail("  pessoa@example.com  "), "pessoa@example.com");
});

test("common single email addresses are valid", () => {
  for (const value of ["", "pessoa@example.com", "nome.sobrenome+tag@sub.example.com.br", "a@b.co"]) {
    assert.equal(validEmail(value), true, value);
  }
});

test("malformed, multiple, display-name and internationalized addresses are invalid", () => {
  const tooLongLocal = `${"a".repeat(65)}@example.com`;
  for (const value of [
    "pessoa", "pessoa@localhost", "pessoa @example.com", "Pessoa <pessoa@example.com>",
    "a@example.com,b@example.com", ".pessoa@example.com", "pessoa..teste@example.com",
    "pessoa@example..com", "pessoa@-example.com", "pessoa@example-.com", "pessoa@exemplo.cöm", tooLongLocal,
  ]) {
    assert.equal(validEmail(value), false, value);
  }
});

test("field validation normalizes values, reports invalid emails and preserves system users", () => {
  const fields = [
    { fieldname: "email", fieldtype: "Email", label: "E-mail" },
    { fieldname: "secondary", fieldtype: "Email", label: "E-mail secundário" },
  ];
  const doc = { email: "  pessoa@example.com ", secondary: "invalid" };

  assert.deepEqual(validateEmailFields(fields, doc, "Pessoa"), { secondary: "E-mail inválido" });
  assert.equal(doc.email, "pessoa@example.com");

  const admin = { email: "Admin" };
  assert.deepEqual(validateEmailFields(fields.slice(0, 1), admin, "User"), {});
});

test("field validation clears errors after correction and stores optional whitespace as null", () => {
  const fields = [
    { fieldname: "email", fieldtype: "Email", label: "E-mail" },
    { fieldname: "secondary", fieldtype: "Email", label: "E-mail secundário" },
  ];
  const doc = { email: "valid@example.com", secondary: "   " };

  assert.deepEqual(validateEmailFields(fields, doc, "Pessoa"), {});
  assert.equal(doc.secondary, null);
});

import { describe, it, expect } from "vitest";
import { describeDevice, isExpired, passwordProblem, sortSessions } from "./profile";

const t = (s: string, args?: any[]) =>
  args ? args.reduce<string>((acc, v, i) => acc.replaceAll(`{${i}}`, String(v)), s) : s;

describe("passwordProblem", () => {
  it("pede uma senha antes de qualquer outra coisa", () => {
    expect(passwordProblem("", "", 8, t)).toBe("Choose a password");
  });

  it("cobra o mínimo do servidor, e diz qual é", () => {
    expect(passwordProblem("curta", "curta", 8, t)).toBe("The password must have at least 8 characters");
    expect(passwordProblem("curta", "curta", 4, t)).toBe("");
  });

  it("só reclama da confirmação depois que a senha já serve", () => {
    // senão a pessoa corrige a confirmação e descobre o tamanho só então
    expect(passwordProblem("abc", "outra", 8, t)).toBe("The password must have at least 8 characters");
    expect(passwordProblem("senhaboa1", "outra", 8, t)).toBe("The two passwords do not match");
  });

  it("aceita o par válido", () => {
    expect(passwordProblem("senhaboa1", "senhaboa1", 8, t)).toBe("");
  });
});

describe("describeDevice", () => {
  it("reconhece o suficiente para a pessoa se reconhecer", () => {
    const chrome = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36";
    expect(describeDevice(chrome, "?")).toBe("Chrome · macOS");

    const iphone = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1";
    expect(describeDevice(iphone, "?")).toBe("Safari · iOS");

    const edge = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36 Edg/120.0";
    expect(describeDevice(edge, "?")).toBe("Edge · Windows");
  });

  it("não inventa quando não sabe", () => {
    expect(describeDevice(null, "Unknown device")).toBe("Unknown device");
    expect(describeDevice("", "Unknown device")).toBe("Unknown device");
    expect(describeDevice("curl/8.4.0", "Unknown device")).toBe("Unknown device");
  });
});

describe("sortSessions", () => {
  it("põe a sessão atual em primeiro, e o resto pelo acesso mais recente", () => {
    const rows = [
      { id: "a", current: false, lastSeen: "2026-09-01T10:00:00Z" },
      { id: "b", current: false, lastSeen: "2026-09-10T10:00:00Z" },
      { id: "c", current: true, lastSeen: "2026-08-01T10:00:00Z" },
    ];
    expect(sortSessions(rows).map((r) => r.id)).toEqual(["c", "b", "a"]);
  });

  it("não mexe no array recebido", () => {
    const rows = [
      { id: "a", current: false, lastSeen: "2026-09-01T10:00:00Z" },
      { id: "c", current: true, lastSeen: "2026-08-01T10:00:00Z" },
    ];
    sortSessions(rows);
    expect(rows.map((r) => r.id)).toEqual(["a", "c"]);
  });
});

describe("isExpired", () => {
  const now = new Date("2026-09-11T12:00:00Z");

  it("uma chave sem expiração nunca está vencida", () => {
    expect(isExpired(null, now)).toBe(false);
    expect(isExpired(undefined, now)).toBe(false);
    expect(isExpired("", now)).toBe(false);
  });

  it("compara com o instante dado", () => {
    expect(isExpired("2026-09-10T12:00:00Z", now)).toBe(true);
    expect(isExpired("2026-09-12T12:00:00Z", now)).toBe(false);
  });

  it("uma data ilegível não vira 'vencida'", () => {
    // marcar como vencida uma chave que funciona confunde mais do que ajuda
    expect(isExpired("nem data é", now)).toBe(false);
  });
});

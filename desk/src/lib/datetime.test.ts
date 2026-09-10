import { describe, it, expect } from "vitest";
import { addDays, addMonths, daysInMonth, fromDatetimeLocal, monthEnd, monthStart, parseDatetime, toDatetimeLocal, today } from "./datetime";

describe("addMonths", () => {
  it("limita o dia ao fim do mês, como o servidor", () => {
    // internal/js/prelude.js: utils.addMonths("2026-01-31", 1) === "2026-02-28"
    expect(addMonths("2026-01-31", 1)).toBe("2026-02-28");
    expect(addMonths("2024-01-31", 1)).toBe("2024-02-29"); // bissexto
    expect(addMonths("2026-03-31", -1)).toBe("2026-02-28");
    expect(addMonths("2026-05-31", 1)).toBe("2026-06-30");
  });
  it("vira o ano nos dois sentidos", () => {
    expect(addMonths("2026-12-15", 1)).toBe("2027-01-15");
    expect(addMonths("2026-01-15", -1)).toBe("2025-12-15");
    expect(addMonths("2026-01-15", -13)).toBe("2024-12-15");
    expect(addMonths("2026-01-15", 25)).toBe("2028-02-15");
  });
  it("não altera a data quando n é zero", () => {
    expect(addMonths("2026-02-28", 0)).toBe("2026-02-28");
  });
  it("aceita um datetime e devolve só a data civil", () => {
    expect(addMonths("2026-01-31T23:30:00Z", 1)).toBe("2026-02-28");
  });
});

describe("addDays", () => {
  it("vira mês e ano", () => {
    expect(addDays("2026-02-28", 1)).toBe("2026-03-01");
    expect(addDays("2024-02-28", 1)).toBe("2024-02-29");
    expect(addDays("2026-12-31", 1)).toBe("2027-01-01");
    expect(addDays("2026-01-01", -1)).toBe("2025-12-31");
  });
});

describe("monthStart / monthEnd", () => {
  it("delimita o mês", () => {
    expect(monthStart("2026-02-17")).toBe("2026-02-01");
    expect(monthEnd("2026-02-17")).toBe("2026-02-28");
    expect(monthEnd("2024-02-01")).toBe("2024-02-29");
    expect(monthEnd("2026-12-05")).toBe("2026-12-31");
  });
  it("sem argumento usa a data local de hoje", () => {
    expect(monthStart()).toBe(today().slice(0, 8) + "01");
    expect(monthEnd().slice(0, 7)).toBe(today().slice(0, 7));
  });
});

describe("daysInMonth", () => {
  it("conhece fevereiro", () => {
    expect(daysInMonth(2026, 1)).toBe(28);
    expect(daysInMonth(2024, 1)).toBe(29);
    expect(daysInMonth(2000, 1)).toBe(29);
    expect(daysInMonth(1900, 1)).toBe(28);
  });
});

describe("today", () => {
  it("é a data civil local, não a UTC", () => {
    // 23:30 local de 31/12 continua sendo 31/12, mesmo que em UTC já seja 01/01
    const d = new Date(2026, 11, 31, 23, 30, 0);
    expect(today(d)).toBe("2026-12-31");
    // e 00:30 local de 01/01 continua sendo 01/01
    expect(today(new Date(2026, 0, 1, 0, 30, 0))).toBe("2026-01-01");
  });
  it("bate com o relógio local agora", () => {
    const n = new Date();
    expect(today()).toBe(`${n.getFullYear()}-${String(n.getMonth() + 1).padStart(2, "0")}-${String(n.getDate()).padStart(2, "0")}`);
  });
});

describe("datetime-local", () => {
  it("mostra o instante em componentes locais e volta igual", () => {
    const local = new Date(2026, 0, 31, 12, 0, 0); // 12:00 na timezone do navegador
    const iso = local.toISOString();
    expect(toDatetimeLocal(iso)).toBe("2026-01-31T12:00");
    expect(fromDatetimeLocal("2026-01-31T12:00")).toBe(iso);
  });
  it("é ida e volta para qualquer instante (sem deslocar horas)", () => {
    for (const d of [new Date(2026, 5, 15, 8, 45), new Date(2026, 11, 1, 23, 59), new Date(2026, 2, 1, 0, 0)]) {
      const iso = d.toISOString();
      expect(fromDatetimeLocal(toDatetimeLocal(iso))).toBe(iso);
    }
  });
  it("trata vazio e inválido", () => {
    expect(toDatetimeLocal(null)).toBe("");
    expect(toDatetimeLocal("")).toBe("");
    expect(toDatetimeLocal("não é data")).toBe("");
    expect(fromDatetimeLocal("")).toBe(null);
    expect(fromDatetimeLocal("xx")).toBe(null);
  });
  it("aceita o formato do Postgres com espaço", () => {
    expect(parseDatetime("2026-01-31 12:00:00")?.getHours()).toBe(12);
  });
});

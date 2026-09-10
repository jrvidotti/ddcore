// dependsOn / readOnlyDependsOn / mandatoryDependsOn evaluator — the same
// semantics the server uses in the goja runtime.
export function evalExpr(expr: string | undefined, doc: any, parent?: any): boolean {
  if (!expr) return true;
  let e = String(expr).trim();
  if (e.startsWith("eval:")) e = e.slice(5);
  if (/^[a-z_][a-z0-9_]*$/.test(e)) return !!doc?.[e];
  try {
    return !!new Function("doc", "parent", "return (" + e + ")")(doc, parent);
  } catch {
    return false;
  }
}

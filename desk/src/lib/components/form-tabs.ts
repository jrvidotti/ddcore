export function resolveActiveTab(tabs: { label?: string }[], param: string | null | undefined): number {
  if (!param || tabs.length <= 1) return 0;
  const p = param.trim().toLowerCase();
  const idx = tabs.findIndex((t) => (t.label || "").trim().toLowerCase() === p);
  return idx >= 0 ? idx : 0;
}

export function tabToSearchParams(tabs: { label?: string }[], activeIndex: number, currentParams?: URLSearchParams): URLSearchParams {
  const params = new URLSearchParams(currentParams);
  if (activeIndex <= 0 || activeIndex >= tabs.length) {
    params.delete("tab");
  } else {
    const label = tabs[activeIndex]?.label;
    if (label) params.set("tab", label);
  }
  return params;
}

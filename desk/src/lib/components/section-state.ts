export function isSectionCollapsed(collapsed: Record<number, boolean>, index: number): boolean {
  return collapsed[index] ?? false;
}

export function toggleSection(collapsed: Record<number, boolean>, index: number): void {
  collapsed[index] = !isSectionCollapsed(collapsed, index);
}

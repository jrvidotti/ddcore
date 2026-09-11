// Panels that hang off a control (the Link typeahead, the date and month
// pickers) used to be `position:absolute` inside the control. Any scrolling
// ancestor — a grid's card, a dialog's body — clipped them. They are now
// placed against the viewport, which nothing clips.

export interface AnchorRect {
  left: number;
  top: number;
  width: number;
  height: number;
}

export interface AnchoredOptions {
  anchor: AnchorRect;
  panel: { width: number; height: number };
  viewport: { width: number; height: number };
  /** Which edge of the panel lines up with the anchor's. Default "start". */
  align?: "start" | "end";
  /** Stretch the panel to the anchor's width (the Link typeahead does). */
  matchWidth?: boolean;
  /** Space between anchor and panel. */
  gap?: number;
  /** Space kept between the panel and the edges of the viewport. */
  margin?: number;
}

export interface AnchoredPosition {
  left: number;
  top: number;
  width: number;
  maxHeight: number;
  placement: "below" | "above";
}

export function computeAnchoredPosition(opts: AnchoredOptions): AnchoredPosition {
  const { anchor, panel, viewport } = opts;
  const gap = opts.gap ?? 4;
  const margin = opts.margin ?? 8;

  const roomBelow = viewport.height - (anchor.top + anchor.height) - gap - margin;
  const roomAbove = anchor.top - gap - margin;
  const placement = panel.height <= roomBelow || roomBelow >= roomAbove ? "below" : "above";

  const width = opts.matchWidth ? anchor.width : panel.width;
  const wanted = opts.align === "end" ? anchor.left + anchor.width - width : anchor.left;
  const left = Math.max(margin, Math.min(wanted, viewport.width - width - margin));

  const maxHeight = Math.max(0, placement === "below" ? roomBelow : roomAbove);
  const height = Math.min(panel.height, maxHeight);
  const top = placement === "below"
    ? anchor.top + anchor.height + gap
    : Math.max(margin, anchor.top - gap - height);

  return { left, top, width, maxHeight, placement };
}

export interface AnchoredParams {
  anchor: HTMLElement | null | undefined;
  align?: "start" | "end";
  matchWidth?: boolean;
  gap?: number;
}

/**
 * Svelte action: pins `node` to `params.anchor` in viewport coordinates and
 * keeps it there while anything scrolls or the window resizes. The node stays
 * where Svelte put it in the DOM — only its layout leaves the flow — so
 * mousedown handlers and block teardown behave as before.
 */
export function anchored(node: HTMLElement, params: AnchoredParams) {
  let current = params;

  function place() {
    const anchor = current.anchor;
    if (!anchor?.isConnected) return;
    const rect = anchor.getBoundingClientRect();

    // scrollHeight is the height the panel *wants*, whatever cap is on it
    // right now — measuring it does not disturb the layout, so the
    // ResizeObserver below settles after one pass instead of oscillating.
    const wanted = node.scrollHeight + (node.offsetHeight - node.clientHeight);
    const pos = computeAnchoredPosition({
      anchor: { left: rect.left, top: rect.top, width: rect.width, height: rect.height },
      panel: { width: node.offsetWidth, height: wanted },
      viewport: { width: document.documentElement.clientWidth, height: window.innerHeight },
      align: current.align,
      matchWidth: current.matchWidth,
      gap: current.gap,
    });

    node.style.position = "fixed";
    node.style.left = `${pos.left}px`;
    node.style.top = `${pos.top}px`;
    node.style.maxHeight = `${Math.min(wanted, pos.maxHeight)}px`;
    if (current.matchWidth) node.style.width = `${pos.width}px`;
    node.dataset.placement = pos.placement;
  }

  place();
  // `capture` so a scroll in any ancestor — the grid card, a dialog — is seen.
  window.addEventListener("scroll", place, true);
  window.addEventListener("resize", place);
  const observer = new ResizeObserver(place);
  observer.observe(node);

  return {
    update(next: AnchoredParams) {
      current = next;
      place();
    },
    destroy() {
      window.removeEventListener("scroll", place, true);
      window.removeEventListener("resize", place);
      observer.disconnect();
    },
  };
}

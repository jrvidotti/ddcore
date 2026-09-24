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
  /** Never narrower than the anchor; wider when the content asks for it (the Link typeahead does). */
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

  const width = opts.matchWidth
    ? Math.min(Math.max(anchor.width, panel.width), viewport.width - 2 * margin)
    : panel.width;
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
  /** Changes when the panel's contents change, so its height is remeasured. */
  content?: unknown;
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

    // Let content changes grow the panel before measuring it. A previous
    // placement may have capped max-height, which keeps ResizeObserver from
    // noticing that scrollHeight increased. The panel's own CSS max-height
    // still caps it, and its scroll position survives the remeasure.
    const scrollTop = node.scrollTop;
    node.style.maxHeight = "";
    const cssMax = parseFloat(getComputedStyle(node).maxHeight); // NaN for "none"
    node.style.maxHeight = "none";
    // Same for the width: measure the content's own width, floored at the
    // anchor's, instead of the width a previous placement fixed.
    if (current.matchWidth) {
      node.style.width = "";
      node.style.minWidth = `${rect.width}px`;
    }
    const natural = node.scrollHeight + (node.offsetHeight - node.clientHeight);
    const wanted = Number.isFinite(cssMax) ? Math.min(natural, cssMax) : natural;
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
    node.scrollTop = scrollTop;
  }

  // The panel scrolling itself is not a reason to move it — and placing it
  // then would throw away the scroll the user just made.
  function onScroll(e: Event) {
    if (e.target instanceof Node && node.contains(e.target)) return;
    place();
  }

  place();
  // `capture` so a scroll in any ancestor — the grid card, a dialog — is seen.
  window.addEventListener("scroll", onScroll, true);
  window.addEventListener("resize", place);
  const observer = new ResizeObserver(place);
  observer.observe(node);

  return {
    update(next: AnchoredParams) {
      current = next;
      place();
    },
    destroy() {
      window.removeEventListener("scroll", onScroll, true);
      window.removeEventListener("resize", place);
      observer.disconnect();
    },
  };
}

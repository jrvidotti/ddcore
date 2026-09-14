# Mobile Layout & Navigation UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide a polished mobile navigation experience with a sticky mobile topbar, drawer backdrop overlay, outside-click dismissal, body scroll locking, and automatic drawer closing on navigation.

**Architecture:** Svelte 5 layout in `desk/src/routes/+layout.svelte` renders the sticky mobile topbar and backdrop overlay, managing `sidebarOpen` state and body scroll locking. `Sidebar.svelte` handles link click dismissal, mobile close button (`X`), and drawer elevation.

**Tech Stack:** Svelte 5, SvelteKit, CSS responsive queries (`@media (max-width: 800px)`), Vitest.

**Spec:** [`docs/superpowers/specs/2026-09-14-mobile-layout-design.md`](file:///Users/junior/dev/ddcore/docs/superpowers/specs/2026-09-14-mobile-layout-design.md)

## Global Constraints
- Desktop layout (`viewport > 800px`) must not be changed or degraded.
- User-facing text must use English and `__("...")`.
- All tests must pass: `npm --prefix desk test` and `npm --prefix desk run check`.

---

### Task 1: Navigation Drawer Backdrop, Outside-Click & Link Dismissal

**Files:**
- Modify: `desk/src/routes/+layout.svelte`
- Modify: `desk/src/lib/components/Sidebar.svelte`

- [ ] **Step 1: Add backdrop overlay and body scroll lock to `+layout.svelte`**
- [ ] **Step 2: Add auto-close on route change to `+layout.svelte`**
- [ ] **Step 3: Add close button and link click dismissal to `Sidebar.svelte`**
- [ ] **Step 4: Verify type checks and tests**
- [ ] **Step 5: Commit**

---

### Task 2: Sticky Mobile Topbar

**Files:**
- Modify: `desk/src/routes/+layout.svelte`

- [ ] **Step 1: Replace floating hamburger button with structured sticky `.mobile-topbar`**
- [ ] **Step 2: Add brand logo, site name, notifications with badge, and user avatar to topbar**
- [ ] **Step 3: Adjust mobile page content padding**
- [ ] **Step 4: Verify type checks and tests**
- [ ] **Step 5: Commit**

---

### Task 3: Verification & Build

**Files:**
- None (verification)

- [ ] **Step 1: Run Desk Vitest suite**
- [ ] **Step 2: Run svelte-check**
- [ ] **Step 3: Build Desk bundle (`npm run build`)**
- [ ] **Step 4: Run Go checks (`make test-go`, `make check`)**
- [ ] **Step 5: Commit any final adjustments**

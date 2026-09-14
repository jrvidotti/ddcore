# Mobile Layout & Navigation Drawer UX Design Specification

**Date:** 2026-09-14  
**Status:** Approved  
**Topic:** Mobile Layout, Sticky Topbar, Drawer Backdrop & Navigation UX  

## 1. Problem Statement

In mobile view (`viewport <= 800px`), the Desk navigation had several usability issues:
1. When tapping any navigation link inside the sidebar drawer, the sidebar did not close, forcing the user to manually find and tap the hamburger button again to see the page.
2. When tapping outside the open sidebar drawer, the menu did not dismiss. There was no backdrop overlay to dim the content or capture outside clicks.
3. There was no sticky/fixed topbar in mobile mode; only a raw hamburger button floating over page content, causing content to scroll beneath the button with poor readability and contrast.

## 2. Proposed Solution & Architecture

### 2.1 Sticky Mobile Topbar (`+layout.svelte`)
- Fixed at the top (`height: 52px; position: sticky; top: 0; background: #fff; border-bottom: 1px solid var(--border); z-index: 45;`).
- Only visible in mobile mode (`@media (max-width: 800px)`).
- **Left:** Accessible hamburger toggle button (`aria-label="Toggle menu"`), showing `menu` or `x`.
- **Center:** Site logo + Site Name (`siteName()`), with an active workspace badge if available.
- **Right:**
  - Quick-access notification icon (`bell`) with badge count for unread notifications.
  - User avatar with initial linking to `/app/profile`.
- Page container padding updated to `padding: 68px 16px 20px` on mobile so no page content is hidden underneath the topbar.

### 2.2 Navigation Drawer Overlay & Backdrop
- A dimmed backdrop element (`.sidebar-backdrop`) rendered behind the sidebar:
  - Background: `rgba(0, 0, 0, 0.45)` with a smooth opacity fade transition.
  - Clicking/tapping the backdrop immediately closes the drawer (`sidebarOpen = false`).
  - Pressing `Escape` key closes the drawer.
- Body scroll locking on mobile while drawer is open:
  - Setting document body `overflow: hidden` when drawer is open on mobile to prevent background scrolling.
- Close button (`X`) in the sidebar header:
  - In mobile view, the sidebar header displays an explicit close button next to the brand title.

### 2.3 Automatic Dismissal on Navigation
- Tapping any navigation link (`<a>`), workspace selector item, or profile item in `Sidebar.svelte` immediately dismisses the drawer if on mobile.
- SvelteKit page navigation watcher (`$effect` tracking `page.url.pathname`) ensures `sidebarOpen = false` whenever navigation occurs.

---

## 3. Verification Plan
- Unit and Svelte diagnostics: `npm --prefix desk test` and `npm --prefix desk run check`.
- Verification of mobile media query rules (`@media (max-width: 800px)`).
- Manual verification on mobile viewport:
  - Topbar renders with hamburger, brand, notifications, avatar.
  - Drawer opens smoothly with backdrop overlay.
  - Clicking backdrop dismisses drawer.
  - Clicking any link in drawer closes drawer and navigates to target.
  - Desktop layout (`> 800px`) remains completely unaffected (topbar hidden, sidebar sticky on left).

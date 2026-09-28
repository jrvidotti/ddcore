<script lang="ts">
  // Geolocation: points, lines and polygons drawn on a map, stored as a GeoJSON
  // FeatureCollection. Leaflet and its stylesheet are loaded on demand, so a
  // form without a map does not pay for them. Drawing is a small hand-written
  // handler — a click adds a point or a vertex, Finish or a double-click closes
  // a line or a polygon — and every finished edit commits the normalized value
  // at once. Tiles come from the site's DDCORE_MAP_TILE_URL.
  import type * as Leaflet from "leaflet";
  import type { Field } from "$lib/meta";
  import { __, boot } from "$lib/boot.svelte";
  import { showError } from "$lib/ui.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import { sanitizeHtml } from "$lib/richtext";
  import { formatGeo, readGeo, type GeoFeatureCollection, type GeoPosition } from "$lib/geo";
  import {
    addGeometry, appendVertex, canFinish, draftGeometry, fromLatLng, removeFeature, toLatLng, type GeoMode,
  } from "./geolocation-state";

  let { field, value, onchange, readOnly = false, error = "", id = "" }:
    { field: Field; value: any; onchange: (v: any) => void; readOnly?: boolean; error?: string; id?: string } = $props();

  const DEFAULT_TILES = { tileUrl: "https://tile.openstreetmap.org/{z}/{x}/{y}.png", attribution: "© OpenStreetMap contributors" };
  const COLOR = "#2563eb";

  const fc = $derived(readGeo(value));
  const invalid = $derived(value !== null && value !== undefined && value !== "" && !fc);
  const summary = $derived(formatGeo(fc));

  let mode = $state<GeoMode>("none");
  let draft = $state<GeoPosition[]>([]);
  let host = $state<HTMLDivElement | null>(null);
  let ready = $state(false);
  let locating = $state(false);
  // only where the browser will actually ask: a secure page with the API
  const canLocate = typeof window !== "undefined" && window.isSecureContext && "geolocation" in navigator;

  let L: typeof Leaflet | null = null;
  let map: Leaflet.Map | null = null;
  let shapes: Leaflet.FeatureGroup | null = null;
  let draftLayer: Leaflet.LayerGroup | null = null;
  // the value this control committed last, as JSON: a different one arriving
  // (a reload, a form script) moves the map to it; our own edit does not.
  // undefined until the first draw, which always places the map: a Leaflet map
  // with no view loads no tiles and ignores clicks, so an empty field needs it too
  let sentKey: string | null | undefined = undefined;

  $effect(() => {
    if (!host) return;
    const el = host;
    let cancelled = false;
    let observer: ResizeObserver | null = null;
    (async () => {
      const mod: any = await import("leaflet");
      await import("leaflet/dist/leaflet.css");
      if (cancelled) return;
      L = (mod.default ?? mod) as typeof Leaflet;
      const tiles = boot.data?.site?.map?.tileUrl ? boot.data.site.map : DEFAULT_TILES;
      // a double-click finishes a shape instead of zooming
      map = L.map(el, { doubleClickZoom: false, worldCopyJump: true });
      L.tileLayer(tiles.tileUrl, {
        maxZoom: 19,
        // the operator's text, but Leaflet writes it as markup
        attribution: sanitizeHtml(tiles.attribution || ""),
      }).addTo(map);
      shapes = L.featureGroup().addTo(map);
      draftLayer = L.layerGroup().addTo(map);
      map.on("click", onMapClick);
      map.on("dblclick", finish);
      // a map laid out while hidden (another tab, a closed section) has no size
      if (typeof ResizeObserver !== "undefined") {
        observer = new ResizeObserver(() => map?.invalidateSize());
        observer.observe(el);
      }
      ready = true;
    })().catch((err) => showError(err));
    return () => {
      cancelled = true;
      observer?.disconnect();
      map?.remove();
      map = shapes = draftLayer = L = null;
      sentKey = undefined;
      ready = false;
    };
  });

  // draw the value, and move the map to it unless it is our own edit
  $effect(() => {
    const current = fc;
    const currentMode = mode;
    if (!ready || !L || !map || !shapes) return;
    shapes.clearLayers();
    current?.features.forEach((f, i) => {
      const g = f.geometry;
      const layers: Leaflet.Layer[] =
        g.type === "Point" ? [marker(g.coordinates)]
        : g.type === "MultiPoint" ? g.coordinates.map(marker)
        : g.type === "LineString" ? [L!.polyline(g.coordinates.map(toLatLng), { color: COLOR, weight: 3 })]
        : [L!.polygon(g.coordinates.map((ring) => ring.map(toLatLng)), { color: COLOR, weight: 2, fillOpacity: 0.15 })];
      for (const layer of layers) {
        layer.on("click", (e: Leaflet.LeafletMouseEvent) => {
          if (currentMode !== "delete") return;
          L!.DomEvent.stopPropagation(e);
          commit(removeFeature(fc, i));
        });
        shapes!.addLayer(layer);
      }
    });
    const key = current ? JSON.stringify(current) : null;
    if (key !== sentKey) {
      sentKey = key;
      fit();
    }
  });

  // the vertices of the line or polygon being drawn
  $effect(() => {
    const vertices = draft;
    const currentMode = mode;
    if (!ready || !L || !draftLayer) return;
    draftLayer.clearLayers();
    if (!vertices.length) return;
    const latlngs = vertices.map(toLatLng);
    if (currentMode === "polygon" && latlngs.length >= 3) {
      L.polygon(latlngs, { color: COLOR, weight: 2, dashArray: "4 4", fillOpacity: 0.08 }).addTo(draftLayer);
    } else if (latlngs.length >= 2) {
      L.polyline(latlngs, { color: COLOR, weight: 3, dashArray: "4 4" }).addTo(draftLayer);
    }
    for (const p of vertices) marker(p).addTo(draftLayer);
  });

  // L.marker's default icon is an image Leaflet looks up relative to its CSS,
  // which a bundler moves: a circle needs no image
  function marker(p: GeoPosition): Leaflet.CircleMarker {
    return L!.circleMarker(toLatLng(p), { radius: 6, color: "#fff", weight: 2, fillColor: COLOR, fillOpacity: 1 });
  }

  function fit() {
    if (!map || !shapes) return;
    const bounds = shapes.getBounds();
    if (!bounds.isValid()) {
      map.setView([20, 0], 2);
    } else if (bounds.getNorthEast().equals(bounds.getSouthWest())) {
      map.setView(bounds.getCenter(), 15);
    } else {
      map.fitBounds(bounds, { padding: [24, 24], maxZoom: 16 });
    }
  }

  function onMapClick(e: Leaflet.LeafletMouseEvent) {
    if (readOnly) return;
    const p = fromLatLng(e.latlng.lat, e.latlng.lng);
    if (mode === "point") commit(addGeometry(fc, { type: "Point", coordinates: p }));
    else if (mode === "line" || mode === "polygon") draft = appendVertex(draft, p);
  }

  function finish() {
    const g = draftGeometry(mode, draft);
    if (!g) return;
    draft = [];
    commit(addGeometry(fc, g));
  }

  // Synchronous on purpose: a finished shape is the edit, as a blur is for a
  // text box.
  function commit(next: GeoFeatureCollection | null | (() => GeoFeatureCollection | null)) {
    try {
      const v = typeof next === "function" ? next() : next;
      sentKey = v ? JSON.stringify(v) : null;
      onchange(v);
    } catch (err) {
      showError(err);
    }
  }

  function setMode(m: GeoMode) {
    mode = mode === m ? "none" : m;
    draft = [];
  }

  function clearAll() {
    draft = [];
    mode = "none";
    commit(null);
  }

  function locate() {
    locating = true;
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        locating = false;
        const p = fromLatLng(pos.coords.latitude, pos.coords.longitude);
        commit(() => addGeometry(fc, { type: "Point", coordinates: p }));
        map?.setView(toLatLng(p), 16);
      },
      (err) => {
        locating = false;
        showError(err.message || __("Your location is not available"));
      },
      { enableHighAccuracy: true, timeout: 15000 },
    );
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === "Escape" && draft.length) {
      e.stopPropagation();
      draft = [];
    }
  }

  const tools: { mode: GeoMode; icon: string; label: string }[] = [
    { mode: "point", icon: "map-pin", label: __("Point") },
    { mode: "line", icon: "spline", label: __("Line") },
    { mode: "polygon", icon: "pentagon", label: __("Polygon") },
    { mode: "delete", icon: "eraser", label: __("Delete") },
  ];
  const hint = $derived(
    mode === "point" ? __("Click the map to add a point")
    : mode === "line" ? __("Click to add each vertex; double-click or Finish to end the line")
    : mode === "polygon" ? __("Click to add each corner; double-click or Finish to close the polygon")
    : mode === "delete" ? __("Click a shape to delete it")
    : "",
  );
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="geolocation" data-fieldname={field.fieldname} data-fieldtype="Geolocation" onkeydown={onKey}>
  {#if !readOnly}
    <div class="geo-toolbar" role="toolbar" aria-label={field.label || __("Map tools")}>
      {#each tools as t (t.mode)}
        <button type="button" class="btn sm geo-tool" class:active={mode === t.mode} aria-pressed={mode === t.mode}
          data-mode={t.mode} title={t.label} onclick={() => setMode(t.mode)}>
          <Icon name={t.icon} size={14} /><span>{t.label}</span>
        </button>
      {/each}
      {#if canFinish(mode, draft)}
        <button type="button" class="btn sm primary geo-finish" onclick={finish}>{__("Finish")}</button>
      {/if}
      {#if draft.length}
        <button type="button" class="btn sm geo-cancel" onclick={() => (draft = [])}>{__("Cancel")}</button>
      {/if}
      <span class="geo-spacer"></span>
      {#if canLocate}
        <button type="button" class="btn sm geo-locate" disabled={locating} onclick={locate} title={__("Use my location")}>
          <Icon name="locate-fixed" size={14} /><span>{__("Use my location")}</span>
        </button>
      {/if}
      {#if value !== null && value !== undefined && value !== ""}
        <button type="button" class="btn sm geo-clear" onclick={clearAll}>{__("Clear")}</button>
      {/if}
    </div>
  {/if}
  <div bind:this={host} {id} class="geo-map" class:error={!!error} class:drawing={mode !== "none" && mode !== "delete"}
    role="application" aria-label={field.label || __("Map")}></div>
  <div class="geo-status muted">
    {#if invalid}{__("Invalid location")}{:else if hint}{hint}{:else if summary}{summary}{:else}{__("No location")}{/if}
  </div>
</div>

<style>
  .geo-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin-bottom: 6px; }
  .geo-tool { display: inline-flex; align-items: center; gap: 4px; }
  .geo-tool.active { background: var(--accent-bg, #dbeafe); border-color: var(--accent, #2563eb); }
  .geo-locate { display: inline-flex; align-items: center; gap: 4px; }
  .geo-spacer { flex: 1; }
  .geo-map {
    height: 300px;
    border: 1px solid var(--border, #ccc);
    border-radius: 6px;
    overflow: hidden;
    /* Leaflet's panes stack up to z-index 1000: keep them under dialogs and menus */
    position: relative;
    z-index: 0;
  }
  .geo-map.error { border-color: var(--red); }
  .geo-map.drawing { cursor: crosshair; }
  .geo-map.drawing :global(.leaflet-grab) { cursor: crosshair; }
  .geo-status { font-size: 12px; margin-top: 4px; min-height: 1em; }
</style>

// Maintenance mode as the desk sees it (PRD-02). The boot says whether the
// site is paused when the desk starts, the `maintenance` event says when that
// changes, and a refused write says so too — whichever arrives first wins,
// because all three describe the same database flag.
export interface MaintenanceInfo { enabled: boolean; reason?: string }

export const maintenance = $state<{ enabled: boolean; reason: string }>({ enabled: false, reason: "" });

export function setMaintenance(info: MaintenanceInfo | null | undefined) {
  maintenance.enabled = !!info?.enabled;
  maintenance.reason = (info?.enabled && info.reason) || "";
}

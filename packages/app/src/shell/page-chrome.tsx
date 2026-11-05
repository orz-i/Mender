import { createContext, useContext, useEffect } from "react";
import { useLocation } from "react-router";

/** Routed pages can supply an actual object name for the application breadcrumb. */
export const PageHeadingContext = createContext<(path: string, heading?: string) => void>(() => {});

export function usePageHeading(heading: string | undefined) {
  const update = useContext(PageHeadingContext);
  const { pathname } = useLocation();

  useEffect(() => {
    if (!heading) return;
    update(pathname, heading);
    return () => update(pathname);
  }, [update, pathname, heading]);
}

/**
 * Child routes share the parent's first segment. Every breadcrumb carries an
 * exact route so users can return without relying on browser history.
 */
export function getPageTrail(pathname: string, root: { to: string; label: string }) {
  const suffix = root.to === "/" ? "" : pathname.slice(root.to.length);
  const segments = suffix.split("/").filter(Boolean);
  if (root.to === "/agents" && segments[0] === "backend" && segments.length === 2) {
    // Preset chooser is a single destination, not a user-facing "backend" category.
    return [{ ...root }, { to: pathname, label: "Agent configuration" }];
  }
  const items: Array<{ to: string; label: string }> = [{ ...root }];
  let to = root.to === "/" ? "" : root.to;
  for (const segment of segments) {
    to += "/" + segment;
    let value = segment;
    try { value = decodeURIComponent(segment); } catch { /* Preserve malformed route segments as text. */ }
    let label: string;
    if (value === "new") {
      label = root.to === "/agents" ? "Configure agent" :
        root.to === "/profiles" ? "Create profile" :
        root.to === "/sandboxes" ? "Create sandbox" : "Create";
    } else {
      const known: Record<string, string> = {
        edit: "Edit profile",
        deliveries: "Deliveries",
        mcp: "MCP",
        skills: "Skills",
        cli: "Local CLI",
        plugin: "Agent Plugin",
        tools: "Tools",
        workspaces: "Workspaces",
        activity: "Activity",
        external: "External plugin",
      };
      label = known[value] ?? value;
    }
    items.push({ to, label });
  }
  return items;
}

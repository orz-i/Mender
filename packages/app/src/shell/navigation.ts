import type { LucideIcon } from "lucide-react";
import {
  BellIcon,
  BlocksIcon,
  BookOpenIcon,
  BotIcon,
  BoxIcon,
  CableIcon,
  GaugeIcon,
  KeyRoundIcon,
  Layers3Icon,
  LayoutDashboardIcon,
  ScrollTextIcon,
  ShieldCheckIcon,
  TerminalIcon,
  WrenchIcon,
} from "lucide-react";

export type NavigationItem = { to: string; label: string; icon: LucideIcon };

export const primaryNavigation: NavigationItem[] = [
  { to: "/", label: "Overview", icon: LayoutDashboardIcon },
  { to: "/tools", label: "Tools", icon: WrenchIcon },
  { to: "/profiles", label: "Profiles", icon: Layers3Icon },
  { to: "/agents", label: "Agents", icon: BotIcon },
  { to: "/sandboxes", label: "Sandboxes", icon: BoxIcon },
  { to: "/skills", label: "Skills", icon: BookOpenIcon },
  { to: "/plugins", label: "Plugin Marketplace", icon: BlocksIcon },
  { to: "/audit", label: "Audit", icon: ScrollTextIcon },
];

export const workbenchNavigation: NavigationItem[] = [
  { to: "/", label: "Overview", icon: LayoutDashboardIcon },
  { to: "/playground", label: "Playground", icon: TerminalIcon },
  { to: "/tools", label: "Tools", icon: WrenchIcon },
  { to: "/profiles", label: "Profiles", icon: Layers3Icon },
  { to: "/agents", label: "Agents", icon: BotIcon },
  { to: "/sandboxes", label: "Sandboxes", icon: BoxIcon },
  { to: "/skills", label: "Skills", icon: BookOpenIcon },
  { to: "/plugins", label: "Plugin Marketplace", icon: BlocksIcon },
  { to: "/audit", label: "Audit", icon: ScrollTextIcon },
];

export const secondaryNavigation: Array<{ label: string; items: NavigationItem[] }> = [
  {
    label: "Connections",
    items: [
      { to: "/tunnels", label: "Tunnels", icon: CableIcon },
      { to: "/credentials", label: "Credentials", icon: KeyRoundIcon },
    ],
  },
  {
    label: "Advanced",
    items: [
      { to: "/approvals", label: "Approvals", icon: BellIcon },
      { to: "/policy", label: "Policy", icon: ShieldCheckIcon },
      { to: "/runtime", label: "Runtime", icon: GaugeIcon },
    ],
  },
];

export const flatNavigation = [
  ...workbenchNavigation,
  ...secondaryNavigation.flatMap((group) => group.items),
];

export const APPEARANCE_STORAGE_KEY = "mender.appearance";
export type Appearance = "light" | "dark" | "system";

export function readAppearance(): Appearance {
  if (typeof window === "undefined") return "light";
  const stored = window.localStorage.getItem(APPEARANCE_STORAGE_KEY);
  return stored === "dark" || stored === "system" ? stored : "light";
}

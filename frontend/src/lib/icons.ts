import LayoutDashboardIcon from "@lucide/svelte/icons/layout-dashboard";
import { icons as brandIcons } from "../brand/icons";

export const icons = { ...brandIcons, dashboard: LayoutDashboardIcon } as const;
export type IconName = keyof typeof icons;

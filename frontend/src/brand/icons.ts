/** Semantic app names keep library details out of the UI. Only used icons are imported. */
import InboxIcon from "@lucide/svelte/icons/inbox";
import FileCogIcon from "@lucide/svelte/icons/file-cog";
import ExternalLinkIcon from "@lucide/svelte/icons/external-link";
import PanelLeftIcon from "@lucide/svelte/icons/panel-left";
import FileTextIcon from "@lucide/svelte/icons/file-text";
import UsersIcon from "@lucide/svelte/icons/users";
import PackageIcon from "@lucide/svelte/icons/package";
import SettingsIcon from "@lucide/svelte/icons/settings";
import PlusIcon from "@lucide/svelte/icons/plus";
import ChevronLeftIcon from "@lucide/svelte/icons/chevron-left";
import ChevronRightIcon from "@lucide/svelte/icons/chevron-right";
import ChevronDownIcon from "@lucide/svelte/icons/chevron-down";
import XIcon from "@lucide/svelte/icons/x";
import Trash2Icon from "@lucide/svelte/icons/trash-2";
import PencilIcon from "@lucide/svelte/icons/pencil";
import SearchIcon from "@lucide/svelte/icons/search";
import FolderIcon from "@lucide/svelte/icons/folder";
import SaveIcon from "@lucide/svelte/icons/save";
import DownloadIcon from "@lucide/svelte/icons/download";
import CheckIcon from "@lucide/svelte/icons/check";
import FileCheckIcon from "@lucide/svelte/icons/file-check";
import RefreshCwIcon from "@lucide/svelte/icons/refresh-cw";

export const icons = {
  received: InboxIcon,
  "page-settings": FileCogIcon,
  open: ExternalLinkIcon,
  sidebar: PanelLeftIcon,
  invoices: FileTextIcon,
  customers: UsersIcon,
  products: PackageIcon,
  settings: SettingsIcon,
  plus: PlusIcon,
  back: ChevronLeftIcon,
  next: ChevronRightIcon,
  down: ChevronDownIcon,
  close: XIcon,
  trash: Trash2Icon,
  edit: PencilIcon,
  search: SearchIcon,
  folder: FolderIcon,
  save: SaveIcon,
  download: DownloadIcon,
  check: CheckIcon,
  checkFile: FileCheckIcon,
  refresh: RefreshCwIcon,
} as const;
export type IconName = keyof typeof icons;

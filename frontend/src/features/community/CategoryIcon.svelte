<script lang="ts">
  import HammerIcon from "@lucide/svelte/icons/hammer";
  import HardHatIcon from "@lucide/svelte/icons/hard-hat";
  import LaptopIcon from "@lucide/svelte/icons/laptop";
  import PaletteIcon from "@lucide/svelte/icons/palette";
  import MegaphoneIcon from "@lucide/svelte/icons/megaphone";
  import PenLineIcon from "@lucide/svelte/icons/pen-line";
  import LightbulbIcon from "@lucide/svelte/icons/lightbulb";
  import BriefcaseIcon from "@lucide/svelte/icons/briefcase";
  import CalculatorIcon from "@lucide/svelte/icons/calculator";
  import TruckIcon from "@lucide/svelte/icons/truck";
  import UtensilsIcon from "@lucide/svelte/icons/utensils";
  import ShoppingBagIcon from "@lucide/svelte/icons/shopping-bag";
  import HeartPulseIcon from "@lucide/svelte/icons/heart-pulse";
  import GraduationCapIcon from "@lucide/svelte/icons/graduation-cap";
  import CameraIcon from "@lucide/svelte/icons/camera";
  import SparklesIcon from "@lucide/svelte/icons/sparkles";

  let { category, size = 16 }: { category: string; size?: number } = $props();
  const icons = {
    handwerk: HammerIcon,
    bau: HardHatIcon,
    it: LaptopIcon,
    design: PaletteIcon,
    marketing: MegaphoneIcon,
    text: PenLineIcon,
    beratung: LightbulbIcon,
    buero: BriefcaseIcon,
    finanzen: CalculatorIcon,
    transport: TruckIcon,
    gastro: UtensilsIcon,
    handel: ShoppingBagIcon,
    gesundheit: HeartPulseIcon,
    bildung: GraduationCapIcon,
    foto: CameraIcon,
  } as const;
  const Component = $derived(icons[category as keyof typeof icons] ?? SparklesIcon);
</script>

<Component {size} stroke-width="1.75" aria-hidden="true" />

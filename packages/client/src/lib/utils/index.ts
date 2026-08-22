import type { PackageIcon } from "$lib/api/types";
import { getLocale } from "$lib/paraglide/runtime";

export function gmIconBuilder(icon: PackageIcon | undefined): string {
	if (icon) return `data:${icon.mime};base64,${icon.data}`;

	return "/icons/game-maker.svg";
}

export function formatRelativeTime(date: string | Date): string {
	const formatter = new Intl.RelativeTimeFormat(getLocale(), {
		numeric: "auto",
	});
	const diff = Date.now() - new Date(date).getTime();

	const units = [
		["year", 31_536_000_000],
		["month", 2_592_000_000],
		["day", 86_400_000],
		["hour", 3_600_000],
		["minute", 60_000],
	] as const;

	for (const [unit, ms] of units) {
		if (Math.abs(diff) >= ms)
			return formatter.format(-Math.round(diff / ms), unit);
	}

	return formatter.format(0, "minute");
}

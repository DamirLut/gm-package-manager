import { env } from '$env/dynamic/public';

export const site = {
	/** Display name shown in the header and page title. */
	name: env.PUBLIC_SITE_NAME || 'Game Maker Package Manager',
	/** Absolute URL or path to the site icon. `null` falls back to the bundled icon. */
	iconUrl: env.PUBLIC_SITE_ICON_URL || null,
	/** Bundled fallback icon served from `static/`. */
	fallbackIconUrl: '/icons/game-maker.svg'
} as const;

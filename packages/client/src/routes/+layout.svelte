<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import type { Pathname } from '$app/types';
	import { site } from '$lib/config';
	import { locales, localizeHref } from '$lib/paraglide/runtime';
	import Header from '../lib/components/header/Header.svelte';

	import '../styles/global.scss';

	let { children } = $props();

	const faviconUrl = $derived(site.iconUrl ?? site.fallbackIconUrl);
</script>

<svelte:head>
	<title>{site.name}</title>
	<link rel="icon" href={faviconUrl} />
</svelte:head>
<Header />

{@render children()}

<div style="display:none">
	{#each locales as locale (locale)}
		<a href={resolve(localizeHref(page.url.pathname, { locale }) as Pathname)}>{locale}</a>
	{/each}
</div>

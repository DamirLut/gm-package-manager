<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import type { Pathname } from '$app/types';
	import { site } from '$lib/config';
	import { m } from '$lib/paraglide/messages';
	import { locales, localizeHref } from '$lib/paraglide/runtime';
	import Header from '../lib/components/header/Header.svelte';
	import type { LayoutProps } from './$types';

	import '../styles/global.scss';

	let { children, data }: LayoutProps = $props();

	const faviconUrl = $derived(site.iconUrl ?? site.fallbackIconUrl);

	const authErrorText = $derived.by(() => {
		switch (page.url.searchParams.get('auth_error')) {
			case 'provider_disabled':
				return m.auth_error_provider();
			case 'state_invalid':
				return m.auth_error_state();
			case 'exchange_failed':
				return m.auth_error_exchange();
			case 'signup_disabled':
				return m.auth_error_signup();
			case 'access_denied':
				return m.auth_error_denied();
			case 'identity_taken':
				return m.auth_error_identity_taken();
			case 'provider_linked':
				return m.auth_error_provider_linked();
			case 'account_suspended':
				return m.auth_error_suspended();
			case 'session_expired':
				return m.auth_error_session_expired();
			case 'internal':
				return m.auth_error_internal();
			default:
				return '';
		}
	});

	// leaving the banner drops the ?auth_error= parameter
	const dismissAuthError = () => goto(page.url.pathname);
</script>

<svelte:head>
	<title>{site.name}</title>
	<link rel="icon" href={faviconUrl} />
</svelte:head>
<Header session={data.session} />

{#if authErrorText}
	<div class="auth-banner" role="alert">
		<p><strong>{m.auth_error_banner_title()}</strong> {authErrorText}</p>
		<button class="close" onclick={dismissAuthError} aria-label={m.error_close()}>×</button>
	</div>
{/if}

{@render children()}

<div style="display:none">
	{#each locales as locale (locale)}
		<a href={resolve(localizeHref(page.url.pathname, { locale }) as Pathname)}>{locale}</a>
	{/each}
</div>

<style lang="scss">
	.auth-banner {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;
		padding: 10px 16px;
		max-width: 1240px;
		width: 100%;
		margin: 16px auto 0;
		border: 1px solid #e5484d;
		border-radius: 5px;
		color: #ffb4b6;
		background: color-mix(in srgb, #e5484d 12%, transparent);
		font-size: 14px;

		p {
			margin: 0;
		}
	}

	.close {
		all: unset;
		cursor: pointer;
		font-size: 18px;
		line-height: 1;
		padding: 2px 6px;
		border-radius: 4px;

		&:hover {
			background: rgba(255, 255, 255, 0.08);
		}
	}
</style>

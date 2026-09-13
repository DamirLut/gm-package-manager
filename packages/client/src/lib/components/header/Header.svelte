<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { m } from '$lib/paraglide/messages';
	import { site } from '$lib/config';
	import { appVersion, githubUrl } from '$lib/package-info';
	import { authorAvatarBuilder } from '$lib/utils';
	import type { Session } from '$lib/api/types';
	import GithubIcon from '$lib/components/uikit/icons/GithubIcon.svelte';
	import PackageSearch from './PackageSearch.svelte';

	let { session }: { session: Session } = $props();

	const user = $derived(session.user);
	const href = resolve('/');
	const iconUrl = $derived(site.iconUrl ?? site.fallbackIconUrl);
	const signInHref = $derived(
		`/-/auth/github/start?redirect=${encodeURIComponent(page.url.pathname)}`
	);
	const avatarUrl = $derived(user ? authorAvatarBuilder(user.avatar_url ?? '', user.username) : '');
</script>

<header>
	<div class="content">
		<a class="logo" {href}>
			<img src={iconUrl} alt={site.name} width="24" height="24" />
		</a>

		<PackageSearch />

		<div class="right">
			{#if user}
				<a class="account" href={resolve('/account')} title={m.account_title()}>
					<img class="avatar" src={avatarUrl} alt={user.username} width={24} height={24} />
					<span class="username">{user.username}</span>
				</a>
			{:else}
				<!-- served by the registry, not a SvelteKit route -->
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
				<a class="signin" href={signInHref}>
					<GithubIcon width={16} height={16} />
					<span>{m.header_sign_in()}</span>
				</a>
			{/if}

			<a class="github" href={githubUrl} target="_blank" rel="noopener noreferrer external">
				<GithubIcon width={22} height={22} />
				<span class="version">{appVersion}</span>
			</a>
		</div>
	</div>
</header>

<style lang="scss">
	header {
		width: 100%;
		height: 64px;
		background: var(--gmui-color-neutral-900);
	}

	.content {
		max-width: 1240px;
		width: 100%;
		height: 100%;
		margin: 0 auto;
		padding: 0 16px;
		display: flex;
		align-items: center;
		gap: 16px;
	}

	.logo {
		display: inline-flex;
		align-items: center;

		img {
			display: block;
			width: 24px;
			height: 24px;
		}
	}

	.right {
		display: inline-flex;
		align-items: center;
		gap: 16px;
		margin-left: auto;
	}

	.signin {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		padding: 6px 12px;
		border: 1px solid var(--gmui-border-default);
		border-radius: 5px;
		color: #fff;
		font-size: 14px;
		font-weight: 600;
		text-decoration: none;
		white-space: nowrap;
		background: var(--gmui-button-bg);
		transition: background-color 100ms ease-in-out;

		&:hover {
			background: var(--gmui-button-bg-hover);
		}
	}

	.account {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		color: #fff;
		font-size: 14px;
		font-weight: 600;
		text-decoration: none;
		white-space: nowrap;

		.avatar {
			display: block;
			width: 24px;
			height: 24px;
			border-radius: 50%;
		}

		&:hover .username {
			text-decoration: underline;
		}
	}

	.github {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		color: #fff;
		font-size: 15px;
		font-weight: 600;
		text-decoration: none;
		white-space: nowrap;
	}

	.version {
		font-variant-numeric: tabular-nums;
	}
</style>

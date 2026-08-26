<script lang="ts">
	import { resolve } from '$app/paths';
	import { site } from '$lib/config';
	import { appVersion, githubUrl } from '$lib/package-info';
	import GithubIcon from '$lib/components/uikit/icons/GithubIcon.svelte';
	import PackageSearch from './PackageSearch.svelte';

	const href = resolve('/');
	const iconUrl = $derived(site.iconUrl ?? site.fallbackIconUrl);
</script>

<header>
	<div class="content">
		<a class="logo" {href}>
			<img src={iconUrl} alt={site.name} width="24" height="24" />
		</a>

		<PackageSearch />

		<a class="github" href={githubUrl} target="_blank" rel="noopener noreferrer external">
			<GithubIcon width={22} height={22} />
			<span class="version">{appVersion}</span>
		</a>
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

	.github {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		margin-left: auto;
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

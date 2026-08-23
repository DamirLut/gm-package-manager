<script lang="ts">
	import { resolve } from '$app/paths';
	import type { Package } from '$lib/api/types';
	import ClockIcon from '$lib/components/uikit/icons/ClockIcon.svelte';
	import LicenseIcon from '$lib/components/uikit/icons/LicenseIcon.svelte';
	import VersionsIcon from '$lib/components/uikit/icons/VersionsIcon.svelte';
	import Card from '$lib/components/uikit/Card.svelte';
	import { m } from '$lib/paraglide/messages';
	import { formatRelativeTime, gmIconBuilder } from '$lib/utils';

	type Props = {
		package: Package;
	};

	let { package: pkg }: Props = $props();

	const href = $derived(resolve('/packages/[...id]', { id: pkg.name }));
</script>

<Card {href}>
	<div class="package-card">
		<div class="icon">
			<img src={gmIconBuilder(pkg.gm.icon)} alt="package icon" width="64" height="64" />
		</div>
		<div class="content">
			<header>
				<strong class="display-name">{pkg.gm.displayName}</strong>
			</header>
			<p class="description">{pkg.gm.shortDescription || pkg.description}</p>
			<footer>
				<span class="meta author">
					<img src={pkg.author.avatar} alt={pkg.author.name} width="16" height="16" />
					{pkg.author.name}
				</span>
				<span class="meta">
					<VersionsIcon />
					{pkg.version}
				</span>
				<span class="meta">
					<ClockIcon />
					{m.published({ time: formatRelativeTime(pkg.time) })}
				</span>
				<span class="meta">
					<LicenseIcon />
					{pkg.license}
				</span>
			</footer>
		</div>
	</div>
</Card>

<style lang="scss">
	.package-card {
		display: flex;
		align-items: center;
		gap: 12px;
	}

	.icon img {
		display: block;
		width: 64px;
		height: 64px;
		border-radius: 5px;
	}

	.content {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	header {
		display: flex;
		align-items: baseline;
		gap: 8px;
	}

	.display-name {
		font-size: 16px;
		font-weight: bold;
	}

	.version {
		color: var(--gmui-color-neutral-400);
		font-size: 13px;
	}

	.description {
		flex: 1;
		font-size: 14px;
	}

	footer {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 20px;
		font-size: 13px;
		color: var(--gmui-color-neutral-400);

		.meta {
			display: inline-flex;
			align-items: center;
			gap: 4px;

			img {
				border-radius: 50%;
			}

			:global(svg) {
				width: 20px;
				height: 20px;
			}
		}
	}
</style>

<script lang="ts">
	import PackageCard from '$lib/components/PackageCard.svelte';
	import { site } from '$lib/config';
	import { m } from '$lib/paraglide/messages';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<main>
	<h1>{m.packages()}</h1>

	<details class="install">
		<summary>{m.home_install_title()}</summary>
		<div class="body">
			<ol>
				<li>{m.home_install_step1()}</li>
				<li>
					{m.home_install_step2()}
					<div class="registry">
						<div class="row">
							<span>{m.home_install_name()}</span>
							<code>{site.name}</code>
						</div>
						<div class="row">
							<span>{m.home_install_url()}</span>
							<code>{data.origin}</code>
						</div>
					</div>
				</li>
			</ol>
			<img src="/package-manager.png" alt={m.home_install_screenshot_alt()} />
		</div>
	</details>

	<ul>
		{#each data.packages as pkg (pkg._id)}
			<li>
				<PackageCard package={pkg} />
			</li>
		{:else}
			<li class="empty">{m.packages_empty()}</li>
		{/each}
	</ul>
</main>

<style lang="scss">
	main {
		padding: 16px;
		display: flex;
		flex-direction: column;
		gap: 24px;
		max-width: 1240px;
		width: 100%;
		margin: 0 auto;
	}

	ul {
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 8px;

		.empty {
			display: flex;
			align-items: center;
			justify-content: center;
			padding: 32px 16px;
			color: var(--gmui-color-neutral-400);
			font-size: 14px;
		}
	}

	.install {
		background: var(--gmui-bg-surface);
		border: 1px solid var(--gmui-border-default);
		border-radius: 8px;

		summary {
			cursor: pointer;
			user-select: none;
			padding: 12px 16px;
			font-weight: 700;
			color: var(--gmui-text-primary);

			&::marker {
				color: var(--gmui-accent-primary);
			}
		}

		.body {
			display: flex;
			flex-direction: column;
			align-items: flex-start;
			gap: 16px;
			padding: 12px 16px 16px;
			border-top: 1px solid var(--gmui-border-default);
		}

		ol {
			margin: 0;
			padding-left: 20px;
			display: flex;
			flex-direction: column;
			gap: 8px;
			color: var(--gmui-text-secondary);
		}

		.registry {
			display: flex;
			flex-direction: column;
			align-items: flex-start;
			gap: 8px;
			margin-top: 8px;

			.row {
				display: flex;
				align-items: center;
				gap: 8px;

				span {
					font-size: 14px;
					color: var(--gmui-text-secondary);
				}

				code {
					background: var(--gmui-bg-surface-muted);
					border: 1px solid var(--gmui-border-default);
					border-radius: 4px;
					padding: 4px 8px;
					font-size: 13px;
					color: var(--gmui-text-primary);
					word-break: break-all;
				}
			}
		}

		img {
			max-width: 100%;
			border: 1px solid var(--gmui-border-default);
			border-radius: 8px;
		}
	}
</style>

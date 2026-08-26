<script lang="ts">
	import Markdown, { denylist } from 'svelte-exmarkdown';
	import { gfmPlugin } from 'svelte-exmarkdown/gfm';
	import { m } from '$lib/paraglide/messages';
	import Tabs from '$lib/components/uikit/Tabs.svelte';
	import Card from '$lib/components/uikit/Card.svelte';
	import { formatRelativeTime } from '$lib/utils';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const latest = $derived(data.sidebar.latest);
	const versionsCount = $derived(Object.keys(data.sidebar.versions).length);
	const versions = $derived(Object.keys(data.sidebar.versions));
	const dependencies = $derived(Object.entries(latest.dependencies ?? {}));

	let activeTab = $state('readme');

	// TODO: READMEs come from untrusted package publishers and should be
	// sanitized with a real XSS-safe pipeline (e.g. rehype-sanitize or DOMPurify
	// with an explicit tag/attribute allowlist, ideally on the server) instead
	// of relying on Svelte text escaping plus this manual img blocklist.
	const readmePlugins = [gfmPlugin(), denylist(['img'])];

	const tabs = [
		{ id: 'readme', label: m.readme() },
		{ id: 'dependencies', label: m.dependencies() },
		{ id: 'versions', label: m.versions() }
	];
</script>

<main>
	<div class="layout">
		<div class="content">
			<Tabs bind:value={activeTab} {tabs} />

			<Card>
				{#if activeTab === 'readme'}
					<div class="readme">
						<Markdown md={data.readme} plugins={readmePlugins} />
					</div>
				{:else if activeTab === 'dependencies'}
					<ul class="list">
						{#each dependencies as [name, range] (name)}
							<li><span>{name}</span><span>{range}</span></li>
						{/each}
					</ul>
				{:else if activeTab === 'versions'}
					<ul class="list">
						{#each versions as version (version)}
							<li>{version}</li>
						{/each}
					</ul>
				{/if}
			</Card>
		</div>

		<aside class="info">
			<Card>
				<section class="stats">
					<header>
						<h1>{latest.gm.displayName}</h1>
						<p class="id">{data.sidebar._id}</p>
						<p class="description">{latest.gm.shortDescription || latest.description}</p>
					</header>

					<h2>Stats</h2>
					<dl>
						<div>
							<dt>Latest version</dt>
							<dd>{data.sidebar['dist-tags'].latest}</dd>
						</div>
						<div>
							<dt>Versions</dt>
							<dd>{versionsCount}</dd>
						</div>
						<div>
							<dt>First published</dt>
							<dd>{formatRelativeTime(data.sidebar.time.created)}</dd>
						</div>
						<div>
							<dt>Last published</dt>
							<dd>{formatRelativeTime(data.sidebar.time.modified)}</dd>
						</div>
						<div>
							<dt>License</dt>
							<dd>{latest.license || '—'}</dd>
						</div>
					</dl>
				</section>
			</Card>
		</aside>
	</div>
</main>

<style lang="scss">
	main {
		padding: 16px;
		display: flex;
		flex-direction: column;
		gap: 12px;
		max-width: 1240px;
		width: 100%;
		margin: 0 auto;
	}

	.layout {
		display: flex;
		align-items: flex-start;
		gap: 24px;
	}

	.content {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.info {
		flex-shrink: 0;
		width: 280px;
		position: sticky;
		top: 16px;

		.stats {
			header {
				display: flex;
				flex-direction: column;
				gap: 2px;
				padding-bottom: 10px;
				margin-bottom: 10px;
				border-bottom: 1px solid var(--gmui-bg-surface-muted);

				h1 {
					font-size: 18px;
					line-height: 26px;
				}

				.id {
					font-size: 13px;
					color: var(--gmui-color-neutral-400);
				}

				.description {
					font-size: 13px;
					margin-top: 6px;
				}
			}

			h2 {
				font-size: 14px;
				margin-bottom: 8px;
			}
		}

		dl {
			display: flex;
			flex-direction: column;
			gap: 6px;
			font-size: 13px;

			div {
				display: flex;
				justify-content: space-between;
				gap: 12px;
			}

			dt {
				color: var(--gmui-color-neutral-400);
			}
		}
	}

	@media (max-width: 800px) {
		.layout {
			flex-direction: column-reverse;
		}

		.info {
			width: 100%;
			position: static;
		}
	}

	.list {
		list-style: none;
		display: flex;
		flex-direction: column;

		li {
			display: flex;
			justify-content: space-between;
			gap: 12px;
			padding: 8px 0;

			& + li {
				border-top: 1px solid var(--gmui-bg-surface-muted);
			}
		}
	}
</style>

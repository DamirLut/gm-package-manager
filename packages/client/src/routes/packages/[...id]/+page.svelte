<script lang="ts">
	import Markdown from 'svelte-exmarkdown';
	import { gfmPlugin } from 'svelte-exmarkdown/gfm';
	import { formatRelativeTime } from '$lib/utils';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const latest = $derived(data.sidebar.latest);
	const versionsCount = $derived(Object.keys(data.sidebar.versions).length);
</script>

<main>
	<header>
		<h1>{latest.gm.displayName}</h1>
		<p>{data.sidebar._id}</p>
		<p>{latest.gm.shortDescription || latest.description}</p>
	</header>

	<section class="stats">
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

	<section class="readme">
		<h2>README</h2>
		<Markdown md={data.readme} plugins={[gfmPlugin()]} />
	</section>
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
</style>

<script lang="ts">
	import PackageCard from '$lib/components/PackageCard.svelte';
	import { m } from '$lib/paraglide/messages';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<main>
	<h1>{m.packages()}</h1>

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
</style>

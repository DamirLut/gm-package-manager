<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { searchPackages } from '$lib/api/client';
	import type { Package } from '$lib/api/types';
	import Input from '$lib/components/uikit/Input.svelte';
	import SearchIcon from '$lib/components/uikit/icons/SearchIcon.svelte';
	import { m } from '$lib/paraglide/messages';
	import SearchResultItem from './SearchResultItem.svelte';

	let query = $state('');
	let suggestions = $state<Package[]>([]);
	let open = $state(false);
	let settled = $state(false);
	let active = $state(0);

	let container = $state<HTMLElement>();
	let seq = 0;
	let timer: ReturnType<typeof setTimeout> | undefined;

	const hasQuery = $derived(query.trim() !== '');

	// Debounced search: every typed character restarts the timer, stale
	// responses are dropped via the sequence counter.
	$effect(() => {
		clearTimeout(timer);

		if (!hasQuery) {
			seq++;
			suggestions = [];
			open = false;
			settled = true;
			return;
		}

		open = true;
		settled = false;
		const mySeq = ++seq;

		timer = setTimeout(async () => {
			try {
				const results = await searchPackages(fetch, query.trim());
				if (mySeq !== seq) return;
				suggestions = results;
				active = 0;
			} catch {
				if (mySeq !== seq) return;
				suggestions = [];
			}
			settled = true;
		}, 150);

		return () => clearTimeout(timer);
	});

	// Keep the highlighted option visible when navigating with the keyboard.
	$effect(() => {
		if (!open) return;
		document.getElementById(`search-option-${active}`)?.scrollIntoView({ block: 'nearest' });
	});

	function goTo(pkg: Package) {
		query = '';
		open = false;
		goto(resolve('/packages/[...id]', { id: pkg.name }));
	}

	function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		if (suggestions.length === 0) return;
		goTo(suggestions[active] ?? suggestions[0]);
	}

	function handleKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			open = false;
			return;
		}
		if (!open || suggestions.length === 0) return;

		if (event.key === 'ArrowDown') {
			event.preventDefault();
			active = (active + 1) % suggestions.length;
		} else if (event.key === 'ArrowUp') {
			event.preventDefault();
			active = (active - 1 + suggestions.length) % suggestions.length;
		}
	}

	function handleOutsideClick(event: MouseEvent) {
		if (container && event.target instanceof Node && !container.contains(event.target)) {
			open = false;
		}
	}
</script>

<svelte:window onclick={handleOutsideClick} />

<div class="search" bind:this={container}>
	<form class="search-form" onsubmit={handleSubmit} role="search">
		<Input
			bind:value={query}
			placeholder={m.search_placeholder()}
			onkeydown={handleKeydown}
			role="combobox"
			aria-expanded={open}
			aria-controls="search-results"
			aria-autocomplete="list"
			aria-activedescendant={open && suggestions.length > 0 ? `search-option-${active}` : undefined}
			autocomplete="off"
			spellcheck="false"
		>
			{#snippet iconBefore()}
				<SearchIcon width={16} height={16} />
			{/snippet}
		</Input>
	</form>

	{#if open}
		<div id="search-results" class="dropdown" role="listbox" aria-label={m.search_placeholder()}>
			{#if suggestions.length === 0}
				{#if settled}
					<div class="empty">{m.search_no_results()}</div>
				{:else}
					<div class="empty loading">…</div>
				{/if}
			{:else}
				{#each suggestions as pkg, index (pkg.name)}
					<SearchResultItem
						{pkg}
						selected={index === active}
						elementId={`search-option-${index}`}
						onselect={() => goTo(pkg)}
						onmouseenter={() => (active = index)}
					/>
				{/each}
			{/if}
		</div>
	{/if}
</div>

<style lang="scss">
	.search {
		flex: 1;
		max-width: 480px;
		position: relative;

		:global(.input-wrapper) {
			width: 100%;
		}
	}

	.dropdown {
		position: absolute;
		top: calc(100% + 6px);
		left: 0;
		right: 0;
		z-index: 50;
		display: flex;
		flex-direction: column;
		max-height: 384px;
		overflow-y: auto;
		background: var(--gmui-bg-surface);
		border: 1px solid var(--gmui-border-default);
		border-radius: 6px;
		box-shadow: 0 8px 24px rgb(0 0 0 / 0.35);
	}

	.empty {
		padding: 12px;
		font-size: 14px;
		color: var(--gmui-color-neutral-400);
		text-align: center;
	}
</style>

<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		href?: string;
		class?: string;
		children?: Snippet;
	};

	let { href, class: className = '', children }: Props = $props();
</script>

<svelte:element this={href ? 'a' : 'div'} class="card {className}" {href}>
	{@render children?.()}
</svelte:element>

<style lang="scss">
	.card {
		display: block;
		background-color: var(--gmui-bg-surface);
		border: 1px solid var(--gmui-border-default);
		border-radius: 5px;
		padding: 12px 16px;
		text-decoration: none;
		color: inherit;

		&:is(a) {
			cursor: pointer;
			transition: border-color ease-in-out 100ms;

			&:hover {
				border-color: var(--gmui-color-accent-600);
			}
		}
	}
</style>

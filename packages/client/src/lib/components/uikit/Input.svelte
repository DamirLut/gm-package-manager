<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLInputAttributes } from 'svelte/elements';

	type Size = 'sm' | 'md' | 'lg';

	type Props = Omit<HTMLInputAttributes, 'size' | 'value'> & {
		size?: Size;
		value?: string;
		iconBefore?: Snippet;
		iconAfter?: Snippet;
	};

	let {
		size = 'md',
		class: className = '',
		value = $bindable(''),
		iconBefore,
		iconAfter,
		...rest
	}: Props = $props();
</script>

<span class="input-wrapper {size} {className}">
	{#if iconBefore}
		<span class="icon before">{@render iconBefore()}</span>
	{/if}
	<input class="input" bind:value {...rest} />
	{#if iconAfter}
		<span class="icon after">{@render iconAfter()}</span>
	{/if}
</span>

<style lang="scss">
	.input-wrapper {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		box-sizing: border-box;
		background-color: var(--gmui-input-bg);
		border: 1px solid var(--gmui-input-border);
		border-radius: 5px;

		transition: border-color 100ms ease-in-out;

		&:focus-within {
			border-color: var(--gmui-input-border-focus);
		}

		.icon {
			display: inline-flex;
			align-items: center;
			flex: 0 0 auto;
			color: var(--gmui-color-neutral-400);
		}

		.input {
			all: unset;
			flex: 1;
			min-width: 0;
			box-sizing: border-box;
			color: inherit;
			font-size: 14px;

			&::placeholder {
				color: var(--gmui-color-neutral-400);
			}
		}

		&:has(.input:disabled) {
			background-color: var(--gmui-button-bg-disabled);
			opacity: 0.6;
		}
	}

	.sm {
		min-height: 24px;
		padding: 0 8px;
	}

	.md {
		min-height: 32px;
		padding: 0 10px;
	}

	.lg {
		min-height: 40px;
		padding: 0 12px;
	}
</style>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';

	type Size = 'sm' | 'md' | 'lg';

	type Props = HTMLButtonAttributes & {
		size?: Size;
		leftIcon?: Snippet;
		rightIcon?: Snippet;
	};

	let {
		class: className = '',
		size = 'md',
		leftIcon,
		rightIcon,
		children,
		...rest
	}: Props = $props();
</script>

<button class="button {size} {className}" {...rest}>
	{@render leftIcon?.()}
	{@render children?.()}
	{@render rightIcon?.()}
</button>

<style lang="scss">
	.button {
		all: unset;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		box-sizing: border-box;
		gap: 6px;
		border-radius: 5px;
		background-color: var(--gmui-button-bg);
		border: 1px solid var(--gmui-button-border);
		text-align: center;
		color: inherit;
		user-select: none;

		transition: background-color 100ms ease-in-out;

		&:hover {
			background-color: var(--gmui-button-bg-hover);
		}

		&:active {
			background-color: var(--gmui-button-bg-active);
		}

		&:disabled {
			background-color: var(--gmui-button-bg-disabled);
		}

		&:not(:disabled) {
			cursor: pointer;
		}
	}

	.sm {
		min-height: 24px;
		padding: 0 10px;
	}

	.md {
		min-height: 32px;
		padding: 1px 15px;
	}

	.lg {
		min-height: 40px;
		padding: 2px 18px;
	}
</style>

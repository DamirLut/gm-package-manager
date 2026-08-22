<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';

	type Size = 'sm' | 'md' | 'lg';
	type Variant = 'solid' | 'ghost';

	type Props = Omit<HTMLButtonAttributes, 'children' | 'aria-label'> & {
		'aria-label': string;
		size?: Size;
		variant?: Variant;
		children: Snippet;
	};

	let {
		class: className = '',
		size = 'md',
		variant = 'solid',
		children,
		...rest
	}: Props = $props();
</script>

<button class="button {variant} {size} {className}" {...rest}>
	{@render children()}
</button>

<style lang="scss">
	.button {
		all: unset;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		box-sizing: border-box;
		border-radius: 5px;
		border: 1px solid transparent;
		color: inherit;
		user-select: none;

		cursor: pointer;

		transition:
			background-color 100ms ease-in-out,
			border-color 100ms ease-in-out;

		&:disabled :global(svg) {
			opacity: 0.5;
		}
	}

	.solid {
		background-color: var(--gmui-button-bg);
		border-color: var(--gmui-button-border);

		&:hover {
			background-color: var(--gmui-button-bg-hover);
		}

		&:active {
			background-color: var(--gmui-button-bg-active);
		}

		&:disabled {
			background-color: var(--gmui-button-bg-disabled);
		}
	}

	.ghost {
		background-color: transparent;
		border-color: transparent;

		&:active {
			background-color: var(--gmui-button-bg-active);
			border-color: var(--gmui-button-border);
		}

		&:disabled {
			background-color: transparent;
			border-color: transparent;
		}
	}

	.sm {
		width: 24px;
		height: 24px;
	}

	.md {
		width: 32px;
		height: 32px;
	}

	.lg {
		width: 40px;
		height: 40px;
	}
</style>

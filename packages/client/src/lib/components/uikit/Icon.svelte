<script lang="ts">
	import type { Snippet } from 'svelte';

	type Size = 'sm' | 'md' | 'lg';

	type Props = {
		size?: Size;
		class?: string;
		children: Snippet;
	};

	let { size = 'md', class: className = '', children }: Props = $props();
</script>

<span class="icon {size} {className}">
	{@render children()}
</span>

<style lang="scss">
	.icon {
		display: inline-block;
		flex-shrink: 0;
		color: currentColor;

		:global(svg) {
			display: block;
			width: 100%;
			height: 100%;
		}
	}

	.sm {
		width: 12px;
		height: 12px;
	}

	.md {
		width: 16px;
		height: 16px;
	}

	.lg {
		width: 20px;
		height: 20px;
	}
</style>

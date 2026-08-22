<script lang="ts">
	import type { SVGAttributes } from 'svelte/elements';

	let { ...rest }: SVGAttributes<SVGSVGElement> = $props();
</script>

<svg
	width="11"
	height="8"
	viewBox="0 0 11 8"
	fill="none"
	xmlns="http://www.w3.org/2000/svg"
	{...rest}
>
	<path
		d="M1.50003 3.99705L3.31025 6.09676"
		stroke="currentColor"
		stroke-width="3"
		stroke-linecap="round"
	/>
	<path
		d="M8.80771 1.5L3.80771 6.5"
		stroke="currentColor"
		stroke-width="3"
		stroke-linecap="round"
	/>
</svg>

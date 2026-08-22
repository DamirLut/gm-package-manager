<script lang="ts">
	import type { SVGAttributes } from 'svelte/elements';

	let { ...rest }: SVGAttributes<SVGSVGElement> = $props();
</script>

<svg
	width="11"
	height="5"
	viewBox="0 0 11 5"
	fill="none"
	xmlns="http://www.w3.org/2000/svg"
	{...rest}
>
	<path
		d="M8.57106 2.53553L1.49999 2.53553"
		stroke="currentColor"
		stroke-width="3"
		stroke-linecap="round"
	/>
</svg>

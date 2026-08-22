<script lang="ts">
	import type { SVGAttributes } from 'svelte/elements';

	let { ...rest }: SVGAttributes<SVGSVGElement> = $props();
</script>

<svg
	width="16"
	height="16"
	viewBox="0 0 16 16"
	fill="none"
	xmlns="http://www.w3.org/2000/svg"
	{...rest}
>
	<path
		d="M4.25 4.25L11.75 11.75M11.75 4.25L4.25 11.75"
		stroke="currentColor"
		stroke-width="1.6"
		stroke-linecap="round"
	/>
</svg>

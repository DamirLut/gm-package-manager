<script lang="ts">
	import type { Package } from '$lib/api/types';
	import { gmIconBuilder } from '$lib/utils';

	type Props = {
		pkg: Package;
		selected: boolean;
		/** Listbox option id, referenced by the input's aria-activedescendant. */
		elementId: string;
		onselect: () => void;
		onmouseenter: () => void;
	};

	let { pkg, selected, elementId, onselect, onmouseenter }: Props = $props();
</script>

<button
	type="button"
	class="option"
	class:selected
	id={elementId}
	role="option"
	aria-selected={selected}
	onmousedown={onselect}
	{onmouseenter}
>
	<img class="icon" src={gmIconBuilder(pkg.gm.icon)} alt="" width="28" height="28" />
	<span class="text">
		<strong>{pkg.gm.displayName || pkg.name}</strong>
		<span class="id">{pkg.name}</span>
		<span class="description">{pkg.gm.shortDescription || pkg.description}</span>
	</span>
</button>

<style lang="scss">
	.option {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 8px 10px;
		width: 100%;
		background: none;
		border: none;
		text-align: left;
		color: inherit;
		font: inherit;
		cursor: pointer;

		&.selected,
		&:hover {
			background: var(--gmui-bg-element);
		}

		.icon {
			flex: 0 0 auto;
			width: 28px;
			height: 28px;
			border-radius: 4px;
		}

		.text {
			flex: 1;
			min-width: 0;
			display: flex;
			flex-direction: column;
			gap: 1px;

			strong {
				font-size: 14px;
				font-weight: 600;
			}

			.id {
				font-size: 12px;
				color: var(--gmui-color-neutral-400);
			}

			.description {
				font-size: 13px;
				color: var(--gmui-text-secondary);
				white-space: nowrap;
				overflow: hidden;
				text-overflow: ellipsis;
			}
		}
	}
</style>

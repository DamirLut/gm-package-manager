<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLInputAttributes } from 'svelte/elements';
	import CheckIcon from './icons/CheckIcon.svelte';
	import CheckIndeterminateIcon from './icons/CheckIndeterminateIcon.svelte';

	export type CheckboxState = boolean | 'mixed';

	type Size = 'sm' | 'md' | 'lg';

	type Props = Omit<HTMLInputAttributes, 'checked' | 'defaultChecked' | 'size' | 'type'> & {
		checked?: CheckboxState;
		defaultChecked?: CheckboxState;
		label?: Snippet;
		size?: Size;
		onchange?: (event: Event) => void;
	};

	let {
		checked,
		defaultChecked = false,
		label,
		size = 'md',
		class: className = '',
		onchange,
		...rest
	}: Props = $props();

	let inputEl: HTMLInputElement | undefined = $state();

	// eslint-disable-next-line svelte/no-unused-svelte-ignore -- warning only fires in svelte-check
	// svelte-ignore state_referenced_locally -- initial value only, like React useState(defaultChecked)
	let uncontrolledChecked = $state<CheckboxState>(defaultChecked);

	let currentChecked = $derived(checked === undefined ? uncontrolledChecked : checked);
	let isMixed = $derived(currentChecked === 'mixed');
	let shouldShowIcon = $derived(currentChecked === true || isMixed);

	$effect(() => {
		if (inputEl) {
			inputEl.indeterminate = isMixed;
		}
	});

	function handleChange(event: Event) {
		if (checked === undefined) {
			uncontrolledChecked = (event.currentTarget as HTMLInputElement).checked;
		}

		onchange?.(event);
	}
</script>

<label class="checkbox-wrapper {size} {className}">
	<span class="checkbox-control">
		<input
			bind:this={inputEl}
			{...rest}
			type="checkbox"
			class="checkbox"
			checked={currentChecked === true}
			aria-checked={isMixed ? 'mixed' : currentChecked === true}
			onchange={handleChange}
		/>
		{#if shouldShowIcon}
			<span class="checkbox-icon">
				{#if isMixed}
					<CheckIndeterminateIcon />
				{:else}
					<CheckIcon />
				{/if}
			</span>
		{/if}
	</span>
	{#if label}
		<span class="label">{@render label()}</span>
	{/if}
</label>

<style lang="scss">
	.checkbox-wrapper {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		color: inherit;
		user-select: none;

		&:has(.checkbox:not(:disabled)) {
			cursor: pointer;
		}

		&:has(.checkbox:disabled) {
			opacity: 0.6;
		}

		.checkbox-control {
			display: inline-flex;
			align-items: center;
			justify-content: center;
			position: relative;
			flex: 0 0 auto;
			color: var(--gmui-checkbox-check-color);
		}

		.checkbox {
			appearance: none;
			box-sizing: border-box;
			background-color: var(--gmui-checkbox-bg);
			border: 1px solid var(--gmui-checkbox-border);
			border-radius: 4px;
			width: 100%;
			height: 100%;
			margin: 0;
			transition: background-color 100ms ease-in-out;

			&:hover {
				background-color: var(--gmui-checkbox-bg-hover);
			}

			&:active {
				background-color: var(--gmui-checkbox-bg-active);
			}

			&:disabled {
				background-color: var(--gmui-checkbox-bg-disabled);
				cursor: default;
			}
		}

		.checkbox-icon {
			position: absolute;
			inset: 0;
			width: 100%;
			height: 100%;
			padding: 4px;
			box-sizing: border-box;
			pointer-events: none;

			:global(svg) {
				display: block;
				width: 100%;
				height: 100%;
			}
		}

		.label {
			line-height: 1;
		}
	}

	.sm .checkbox-control {
		width: 16px;
		height: 16px;
	}

	.md .checkbox-control {
		width: 20px;
		height: 20px;
	}

	.lg .checkbox-control {
		width: 24px;
		height: 24px;
	}
</style>

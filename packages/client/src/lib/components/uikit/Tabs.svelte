<script lang="ts">
	type Tab = {
		id: string;
		label: string;
	};

	type Props = {
		tabs: Tab[];
		value?: string;
	};

	let { tabs, value = $bindable(tabs[0]?.id ?? '') }: Props = $props();
</script>

<div class="tabs" role="tablist">
	{#each tabs as tab (tab.id)}
		<button
			type="button"
			class="tab"
			class:active={tab.id === value}
			role="tab"
			aria-selected={tab.id === value}
			onclick={() => (value = tab.id)}
		>
			{tab.label}
		</button>
	{/each}
</div>

<style lang="scss">
	.tabs {
		display: flex;
		gap: 4px;
		border-bottom: 1px solid var(--gmui-border-default);
	}

	.tab {
		all: unset;
		box-sizing: border-box;
		padding: 6px 14px;
		font-weight: 700;
		color: var(--gmui-color-neutral-400);
		border-bottom: 2px solid transparent;
		cursor: pointer;

		&:hover,
		&.active {
			color: inherit;
		}

		&.active {
			border-bottom-color: var(--gmui-accent-primary);
		}
	}
</style>

<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { m } from '$lib/paraglide/messages';

	const status = $derived(page.status);
	const notFound = $derived(status === 404);
	const title = $derived(notFound ? m.error_not_found_title() : m.error_generic_title());
	const description = $derived(
		notFound ? m.error_not_found_description() : m.error_generic_description()
	);
	const homeHref = resolve('/');

	function reload() {
		location.reload();
	}

	function goHome() {
		location.href = homeHref;
	}
</script>

<main class="error-page">
	<div class="dialog">
		<div class="dialog__titlebar">
			<span class="dialog__title">{m.error_window_title()} {status}</span>
			<button class="dialog__close" aria-label={m.error_close()} onclick={goHome}>×</button>
		</div>

		<div class="dialog__body">
			<div class="dialog__message">
				<div class="dialog__warning">
					<svg viewBox="0 0 24 24">
						<path d="M12 3L22 20H2L12 3Z" />
						<path d="M12 9V14" />
						<circle cx="12" cy="17" r=".8" />
					</svg>
				</div>

				<div class="dialog__text">
					<span class="dialog__title-text">{title}</span>
					<span class="dialog__desc">{description}</span>
				</div>
			</div>

			<div class="dialog__actions">
				<a class="dialog__button" href={homeHref}>{m.error_back_home()}</a>
				<button class="dialog__button" onclick={reload}>{m.error_try_again()}</button>
			</div>
		</div>
	</div>
</main>

<style lang="scss">
	.error-page {
		min-height: calc(100dvh - 64px);
		display: flex;
		align-items: center;
		justify-content: center;
		padding: 16px;
	}

	.dialog {
		position: relative;
		width: 390px;
		min-height: 150px;
		box-sizing: border-box;
		background: var(--gmui-dialog-bg);
		border: 1px solid var(--gmui-dialog-border);
		border-radius: 6px;
		color: var(--gmui-text-primary);
		font-size: 12px;
		box-shadow: 0 8px 20px rgba(0, 0, 0, 0.35);
	}

	.dialog__titlebar {
		position: relative;
		height: 20px;
		display: flex;
		align-items: center;
		padding-left: 8px;
		border-radius: 6px 6px 0 0;
		background: var(--gmui-dialog-bg);
		overflow: hidden;
	}

	.dialog__title {
		position: relative;
		z-index: 2;
		font-size: 12px;
		line-height: 20px;
		color: var(--gmui-text-primary);
	}

	.dialog__close {
		position: absolute;
		z-index: 3;
		right: 5px;
		top: 1px;
		width: 18px;
		height: 18px;
		padding: 0;
		border: 0;
		background: transparent;
		color: var(--gmui-color-neutral-400);
		font-size: 17px;
		line-height: 16px;
		cursor: pointer;
	}

	.dialog__close:hover {
		color: var(--gmui-text-secondary);
	}

	.dialog__body {
		position: relative;
		min-height: 129px;
		padding: 26px 19px 12px;
		box-sizing: border-box;
	}

	.dialog__message {
		display: flex;
		align-items: center;
		gap: 14px;
		min-height: 40px;
	}

	.dialog__warning {
		flex: 0 0 auto;
		width: 27px;
		height: 27px;
	}

	.dialog__warning svg {
		width: 100%;
		height: 100%;
		fill: none;
		stroke: var(--gmui-color-warning);
		stroke-width: 1.7;
		stroke-linejoin: round;
		stroke-linecap: round;
	}

	.dialog__warning circle {
		fill: var(--gmui-color-warning);
		stroke: none;
	}

	.dialog__text {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.dialog__title-text {
		color: var(--gmui-text-primary);
		font-size: 12px;
		line-height: 16px;
	}

	.dialog__desc {
		color: var(--gmui-text-secondary);
		font-size: 12px;
		line-height: 16px;
	}

	.dialog__actions {
		position: absolute;
		right: 14px;
		bottom: 25px;
		display: flex;
		gap: 8px;
	}

	.dialog__button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 80px;
		height: 24px;
		padding: 0;
		border: 1px solid var(--gmui-button-border);
		border-radius: 3px;
		background: var(--gmui-button-bg);
		color: var(--gmui-text-primary);
		font-size: 12px;
		text-decoration: none;
		cursor: pointer;
	}

	.dialog__button:hover {
		background: var(--gmui-button-bg-hover);
	}

	.dialog__button:active {
		background: var(--gmui-button-bg-active);
	}
</style>

<script lang="ts">
	import { invalidateAll, goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { m } from '$lib/paraglide/messages';
	import { deleteAccount, logout, unlinkIdentity } from '$lib/api/client';
	import { authorAvatarBuilder, formatRelativeTime } from '$lib/utils';
	import Button from '$lib/components/uikit/Button.svelte';
	import Card from '$lib/components/uikit/Card.svelte';
	import Input from '$lib/components/uikit/Input.svelte';
	import GithubIcon from '$lib/components/uikit/icons/GithubIcon.svelte';
	import type { AccountEvent, LinkedIdentity } from '$lib/api/types';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const user = $derived(data.session.user);

	const githubIdentity = $derived(
		user?.identities.find((identity) => identity.provider === 'github')
	);
	// served by the registry, not a SvelteKit route — resolve() does not apply
	const connectHref = '/-/auth/github/start?redirect=' + encodeURIComponent(resolve('/account'));

	let busy = $state(false);
	let actionError = $state('');
	let deleteConfirmation = $state('');

	const providerLabel = (provider?: string) =>
		provider === 'password'
			? m.provider_password()
			: provider === 'github'
				? m.provider_github()
				: (provider ?? '');

	const providerName = (identity: LinkedIdentity) => providerLabel(identity.provider);

	const eventLabel = (event: AccountEvent) =>
		({
			login: m.event_login(),
			identity_linked: m.event_identity_linked(),
			identity_unlinked: m.event_identity_unlinked(),
			package_publish: m.event_package_publish(),
			package_unpublish: m.event_package_unpublish()
		})[event.type] ?? event.type;

	async function run(action: () => Promise<void>) {
		if (busy) return;
		busy = true;
		actionError = '';
		try {
			await action();
		} catch {
			actionError = m.account_action_failed();
		} finally {
			busy = false;
		}
	}

	const disconnect = (provider: string) =>
		run(async () => {
			await unlinkIdentity(fetch, provider);
			await invalidateAll();
		});

	const signOut = () =>
		run(async () => {
			await logout(fetch);
			deleteConfirmation = '';
			await goto(resolve('/'));
		});

	const deleteAccountConfirmed = () =>
		run(async () => {
			if (!user) return;
			await deleteAccount(fetch, user.username);
			deleteConfirmation = '';
			await goto(resolve('/'));
		});
</script>

<main>
	{#if user}
		<header class="page-header">
			<div class="who">
				<img
					class="avatar"
					src={authorAvatarBuilder(user.avatar_url ?? '', user.username)}
					alt={user.username}
					width={56}
					height={56}
				/>
				<div>
					<h1>{user.username}</h1>
					<p class="muted">
						{m.account_member_since({ time: formatRelativeTime(user.created_at) })}
					</p>
				</div>
			</div>
			<Button size="sm" onclick={signOut} disabled={busy}>{m.account_logout()}</Button>
		</header>

		{#if actionError}
			<p class="action-error" role="alert">{actionError}</p>
		{/if}

		<div class="layout">
			<section>
				<h2>{m.account_login_methods()}</h2>
				<Card>
					<ul class="methods">
						{#each user.identities as identity (identity.provider)}
							<li>
								<span class="provider">
									{#if identity.provider === 'github'}
										<GithubIcon width={18} height={18} />
									{/if}
									<span class="provider-name">
										{providerName(identity)}
										<span class="muted">{identity.username}</span>
									</span>
								</span>
								<span class="row-end">
									<span class="muted linked-at">
										{m.account_linked_at({ time: formatRelativeTime(identity.created_at) })}
									</span>
									<Button size="sm" onclick={() => disconnect(identity.provider)} disabled={busy}>
										{m.account_disconnect()}
									</Button>
								</span>
							</li>
						{/each}
					</ul>
					{#if !githubIdentity}
						<!-- served by the registry, not a SvelteKit route -->
						<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
						<a class="connect" href={connectHref}>
							<GithubIcon width={14} height={14} />
							{m.account_connect_github()}
						</a>
					{/if}
					<p class="muted hint">{m.account_login_methods_hint()}</p>
				</Card>
			</section>

			<section>
				<h2>{m.account_login_history()}</h2>
				<Card>
					<ul class="feed">
						{#each data.logins as event (event.created_at + event.provider)}
							<li>
								<span class="what">{eventLabel(event)}</span>
								<span class="muted provider-tag">{providerLabel(event.provider)}</span>
								<span class="muted">{event.ip}</span>
								<span class="muted when">{formatRelativeTime(event.created_at)}</span>
							</li>
						{:else}
							<li class="empty">{m.account_empty_logins()}</li>
						{/each}
					</ul>
				</Card>
			</section>

			<section>
				<h2>{m.account_activity()}</h2>
				<Card>
					<ul class="feed">
						{#each data.activity as event (event.created_at + event.type + event.detail)}
							<li>
								<span class="what">
									{eventLabel(event)}
									{#if event.detail}
										<span class="detail">{event.detail}</span>
									{/if}
								</span>
								<span class="muted when">{formatRelativeTime(event.created_at)}</span>
							</li>
						{:else}
							<li class="empty">{m.account_empty_activity()}</li>
						{/each}
					</ul>
				</Card>
			</section>

			<section class="danger">
				<h2>{m.account_danger_zone()}</h2>
				<Card>
					<h3>{m.account_delete_title()}</h3>
					<p class="muted">{m.account_delete_description()}</p>
					<div class="delete-row">
						<Input
							size="sm"
							placeholder={m.account_delete_confirm_label({ username: user.username })}
							bind:value={deleteConfirmation}
						/>
						<Button
							size="sm"
							class="danger"
							disabled={busy || deleteConfirmation !== user.username}
							onclick={deleteAccountConfirmed}
						>
							{m.account_delete_confirm()}
						</Button>
					</div>
				</Card>
			</section>
		</div>
	{/if}
</main>

<style lang="scss">
	main {
		padding: 16px;
		display: flex;
		flex-direction: column;
		gap: 24px;
		max-width: 1240px;
		width: 100%;
		margin: 0 auto;
	}

	.page-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 16px;

		.who {
			display: flex;
			align-items: center;
			gap: 14px;

			h1 {
				font-size: 22px;
				line-height: 28px;
			}
		}

		.avatar {
			display: block;
			width: 56px;
			height: 56px;
			border-radius: 50%;
		}
	}

	.action-error {
		margin: 0;
		padding: 10px 14px;
		border: 1px solid #e5484d;
		border-radius: 5px;
		color: #ffb4b6;
		background: color-mix(in srgb, #e5484d 12%, transparent);
		font-size: 14px;
	}

	.layout {
		display: flex;
		flex-direction: column;
		gap: 32px;
	}

	section {
		display: flex;
		flex-direction: column;
		gap: 10px;

		h2 {
			font-size: 15px;
		}
	}

	.methods {
		list-style: none;
		display: flex;
		flex-direction: column;

		li {
			display: flex;
			justify-content: space-between;
			align-items: center;
			gap: 12px;
			padding: 10px 0;

			& + li {
				border-top: 1px solid var(--gmui-border-default);
			}
		}

		.provider {
			display: inline-flex;
			align-items: center;
			gap: 10px;
		}

		.provider-name {
			display: inline-flex;
			flex-direction: column;
			font-weight: 600;
		}

		.row-end {
			display: inline-flex;
			align-items: center;
			gap: 12px;
		}

		.linked-at {
			font-size: 13px;
		}

		li.empty {
			justify-content: flex-start;
			color: var(--gmui-color-neutral-400);
		}
	}

	.connect {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		margin-top: 12px;
		width: fit-content;
		padding: 5px 12px;
		border: 1px solid var(--gmui-border-default);
		border-radius: 5px;
		background: var(--gmui-button-bg);
		color: inherit;
		font-size: 14px;
		font-weight: 600;
		text-decoration: none;
		transition: background-color 100ms ease-in-out;

		&:hover {
			background: var(--gmui-button-bg-hover);
		}
	}

	.hint {
		margin: 10px 0 0;
		font-size: 13px;
	}

	.feed {
		list-style: none;
		display: flex;
		flex-direction: column;

		li {
			display: flex;
			justify-content: space-between;
			align-items: center;
			gap: 12px;
			padding: 8px 0;
			font-size: 14px;

			& + li {
				border-top: 1px solid var(--gmui-border-default);
			}
		}

		.what {
			display: inline-flex;
			align-items: center;
			gap: 8px;
			min-width: 0;

			.detail {
				color: var(--gmui-accent-primary);
			}
		}

		.provider-tag {
			flex-shrink: 0;
			font-size: 13px;
		}

		.when {
			flex-shrink: 0;
			font-size: 13px;
		}

		li.empty {
			justify-content: flex-start;
			color: var(--gmui-color-neutral-400);
		}
	}

	.danger {
		h3 {
			font-size: 14px;
		}

		p {
			font-size: 13px;
			margin: 0 0 12px;
		}

		.delete-row {
			display: flex;
			align-items: center;
			gap: 8px;
			flex-wrap: wrap;

			.input-wrapper {
				min-width: 280px;
			}
		}

		:global(.danger) {
			color: #ff8f91;
			border-color: #e5484d;

			&:hover:not(:disabled) {
				background: color-mix(in srgb, #e5484d 20%, transparent);
			}
		}
	}

	.muted {
		color: var(--gmui-text-secondary);
	}
</style>

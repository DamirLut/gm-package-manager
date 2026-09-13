import { redirect } from '@sveltejs/kit';
import { getAccountActivity, getAccountLogins } from '$lib/api/client';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, parent }) => {
	const { session } = await parent();
	if (!session.user) {
		redirect(307, '/');
	}

	const [logins, activity] = await Promise.all([
		getAccountLogins(fetch),
		getAccountActivity(fetch)
	]);

	return { logins, activity };
};

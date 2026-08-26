import pkg from '../../package.json';

const repositoryUrl =
	typeof pkg.repository === 'object' && pkg.repository?.url
		? pkg.repository.url.replace(/\.git$/, '')
		: 'https://github.com/damirlut/gm-package-manager';

export const appVersion = pkg.version;

export const githubUrl = repositoryUrl;

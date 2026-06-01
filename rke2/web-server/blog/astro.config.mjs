// @ts-check

import mdx from '@astrojs/mdx';
import sitemap from '@astrojs/sitemap';
import { defineConfig, fontProviders } from 'astro/config';

// Zero-dependency remark plugin: count words in the post body and inject a
// `minutesRead` string into each post's frontmatter at build time. Applies to
// Markdown and (via @astrojs/mdx's inherited markdown config) MDX.
function remarkReadingTime() {
	return function (tree, file) {
		let words = 0;
		const walk = (node) => {
			if (typeof node.value === 'string') {
				words += node.value.split(/\s+/).filter(Boolean).length;
			}
			if (node.children) node.children.forEach(walk);
		};
		walk(tree);
		const minutes = Math.max(1, Math.round(words / 200));
		file.data.astro.frontmatter.minutesRead = `${minutes} min read`;
	};
}

// https://astro.build/config
export default defineConfig({
	site: 'https://blog.hwcopeland.net',
	integrations: [mdx(), sitemap()],
	devToolbar: { enabled: false },
	markdown: {
		remarkPlugins: [remarkReadingTime],
		shikiConfig: {
			theme: 'github-dark-dimmed',
			wrap: true,
		},
	},
	fonts: [
		{
			provider: fontProviders.local(),
			name: 'Atkinson',
			cssVariable: '--font-atkinson',
			fallbacks: ['sans-serif'],
			options: {
				variants: [
					{
						src: ['./src/assets/fonts/atkinson-regular.woff'],
						weight: 400,
						style: 'normal',
						display: 'swap',
					},
					{
						src: ['./src/assets/fonts/atkinson-bold.woff'],
						weight: 700,
						style: 'normal',
						display: 'swap',
					},
				],
			},
		},
	],
});

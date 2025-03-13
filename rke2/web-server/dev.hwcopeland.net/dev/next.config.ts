const withMDX = require('@next/mdx')({
  options: {
    remarkPlugins: ["remark-math"],
    rehypePlugins: ["rehype-katex"],
  },
})
module.exports = withMDX()

/** @type {import('@docusaurus/types').DocusaurusConfig} */
module.exports = {
  title: 'DGate API Gateway',
  tagline: 'Open Source Function Native API Gateway!',
  url: 'https://dgate.io',
  baseUrl: '/',
  onBrokenLinks: 'warn',
  onBrokenMarkdownLinks: 'warn',
  organizationName: 'dgate-io',
  projectName: 'dgate',
  themes: ['@docusaurus/theme-mermaid'],
  markdown: {
    mermaid: true,
  },
  headTags: [
    {
      tagName: 'meta',
      attributes: {
        property: 'og:image',
        content: '/img/dgate2.png',
      },
    },
    {
      tagName: 'meta',
      attributes: {
        property: 'og:title',
        content: 'DGate API Gateway',
      },
    },
    {
      tagName: 'meta',
      attributes: {
        property: 'og:url',
        content: 'https://dgate.io',
      },
    },
    {
      tagName: 'meta',
      attributes: {
        property: 'og:type',
        content: 'website',
      },
    },
    {
      tagName: 'meta',
      attributes: {
        property: 'og:description',
        content: 'DGate is an open-source, function-native API Gateway that enables you to create, secure, and manage APIs for your applications!',
      },
    },
    {
      tagName: 'meta',
      attributes: {
        name: 'og:keywords',
        content: 'api gateway, open source, serverless, function-native, dgate, dgate.io',
      },
    }
  ],
  themeConfig: {
    docs: {
      sidebar: {
        hideable: true,
      }
    },
    colorMode: {
      defaultMode: 'dark',
      respectPrefersColorScheme: true,
    },
    prism: {
      mermaid: true,
      additionalLanguages: ['javascript', 'typescript', 'bash', 'yaml', 'json'],
    },
    algolia: {
      appId: '7ZF6OL4UBL',
      apiKey: "c7c19d3cedc03f92311ee53a9303cd23",
      indexName: "dgate-io-crawler",
      contextualSearch: true,
    },
    navbar: {
      title: 'DGate',
      hideOnScroll: true,
      logo: {
        alt: 'DGate',
        src: '/img/dgate.svg',
        width: '32px',
        height: '22px',
      },
      items: [
        {
          href: 'https://github.com/dgate-io/dgate',
          className: 'header-github-link',
          position: 'right',
        },
        // {
        //   href: 'https://discord.gg/sZs2NVEgRt',
        //   className: 'header-discord-link',
        //   position: 'right',
        // },
        {
          to: '/docs/intro',
          label: 'Documentation',
          position: 'left',
        },
        {
          to: '/docs/getting-started/dgate-server#installation',
          label: 'Download',
          position: 'left',
        },
        {
          to: 'https://github.com/dgate-io/dgate',
          label: 'Source Code',
          position: 'left',
        },
      ],
    },
    footer: {
      links: [
        {
          label: "Getting Started",
          to: "/docs/getting-started",
        },
        {
          label: "CLI",
          to: "/docs/getting-started/dgate-cli",
        },
        {
          label: "Releases",
          to: "https://github.com/dgate-io/dgate/releases",
        },
        {
          label: "Containers",
          to: "https://github.com/dgate-io/dgate/pkgs/container/dgate",
        },
      ],
      copyright: ` Copyright © ${new Date().getFullYear()} | DGate.io`,
    },
    announcementBar: {
      id: 'ab-source-link-1', // Increment on change
      content: `<strong>DGate API Gateway: check out the newly open-sourced API Gateway! </strong><a class="cta" href="https://github.com/dgate-io/dgate">HERE</a>`,
      isCloseable: true,
    },
  },
  plugins: [
    [
      '@docusaurus/plugin-ideal-image',
      {
        quality: 70,
        max: 1030,
        min: 640,
        steps: 2,
        disableInDev: false,
      },
    ],
  ],
  presets: [
    [
      '@docusaurus/preset-classic',
      {
        docs: {
          routeBasePath: '/docs',
          sidebarPath: require.resolve('./sidebars.js'),
          path: "./docs",
          showLastUpdateAuthor: false,
          showLastUpdateTime: false,
        },
        gtag: {
          trackingID: 'GTM-526DPZWG',
        },
        sitemap: {
          lastmod: 'datetime',
          changefreq: 'weekly',
          ignorePatterns: ['/tags/**'],
          filename: 'sitemap.xml',
        },
        theme: {
          customCss: './src/css/custom.css',
        },
      },
    ],
  ],
};

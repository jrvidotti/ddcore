import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'ddcore',
  description: 'ddcore (Data Driven Core) — Build business applications from TypeScript models in Go, TypeScript, and PostgreSQL',
  base: process.env.VITEPRESS_BASE || '/',
  srcExclude: [
    'superpowers/**',
    'outreach-plan.md',
    'frappe-port-inventory.md',
    'frappe-rest-gap-analysis.md',
    'frappe-implemented-features.md',
  ],
  ignoreDeadLinks: false,
  themeConfig: {
    siteTitle: 'ddcore',
    nav: [
      { text: 'Get Started', link: '/guide/first-app' },
      { text: 'Documentation', link: '/agent/' },
      { text: 'Architecture', link: '/guide/architecture' },
      {
        text: 'Demo',
        items: [
          { text: 'Live Demo', link: 'https://demo.ddcore.dev' },
          { text: 'Demo Walkthrough', link: '/guide/demo' },
          { text: 'Demo Source Code', link: 'https://github.com/jrvidotti/ddcore-demo' }
        ]
      },
      { text: 'GitHub', link: 'https://github.com/jrvidotti/ddcore' }
    ],
    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Build Your First App', link: '/guide/first-app' },
          { text: 'Architecture & Mental Model', link: '/guide/architecture' },
          { text: 'Example App Walkthrough', link: '/guide/demo' },
          { text: 'Deployment & Production', link: '/guide/deployment' }
        ]
      },
      {
        text: 'Reference Overview',
        items: [
          { text: 'Overview', link: '/agent/' },
          { text: 'Conventions & Structure', link: '/agent/conventions' },
          { text: 'CLI & Development Loop', link: '/agent/cli' },
          { text: 'Extending DocTypes', link: '/agent/extending' }
        ]
      },
      {
        text: 'Core Models & APIs',
        items: [
          { text: 'Fieldtypes Reference', link: '/agent/fieldtypes' },
          { text: 'Server Controller API', link: '/agent/controller-api' },
          { text: 'Desk & Form API', link: '/agent/form-api' },
          { text: 'Reports & Workspaces', link: '/agent/report-api' },
          { text: 'Schema Migrations', link: '/agent/migrations' }
        ]
      },
      {
        text: 'Security & Governance',
        items: [
          { text: 'Authentication & Passwords', link: '/agent/auth' },
          { text: 'Field Permissions', link: '/agent/field-permissions' },
          { text: 'User Access Scopes', link: '/agent/scopes' },
          { text: 'Credential Vault', link: '/agent/vault' },
          { text: 'Administrative Audit Trail', link: '/agent/audit' }
        ]
      },
      {
        text: 'Business Logic & Automation',
        items: [
          { text: 'Approval Workflows', link: '/agent/workflows' },
          { text: 'Assignments & ToDos', link: '/agent/assignments' },
          { text: 'Event Notifications', link: '/agent/notifications' },
          { text: 'Email Templates & Delivery', link: '/agent/mail' },
          { text: 'Outgoing Webhooks', link: '/agent/webhooks' },
          { text: 'Print Templates & PDF', link: '/agent/print' },
          { text: 'Data Export', link: '/agent/export' }
        ]
      },
      {
        text: 'Operations & Production',
        items: [
          { text: 'Ops, Health & Observability', link: '/agent/ops' },
          { text: 'Internationalization (i18n)', link: '/agent/i18n' },
          { text: 'Upstream Feature Requests', link: '/agent/feature-requests' }
        ]
      }
    ],
    search: {
      provider: 'local'
    },
    socialLinks: [
      { icon: 'github', link: 'https://github.com/jrvidotti/ddcore' }
    ],
    footer: {
      message: 'MIT Licensed · ddcore (Data Driven Core) · Development documentation (main)',
      copyright: 'Copyright © 2026 ddcore contributors'
    }
  }
})

import { createRouter, createWebHashHistory } from 'vue-router'

const routes = [
  { path: '/', component: () => import('../components/Dashboard/Index.vue'), meta: { nav: 'dashboard' } },
  { path: '/providers', component: () => import('../components/Providers/Index.vue'), meta: { nav: 'providers' } },
  {
    path: '/providers/archived',
    component: () => import('../components/Providers/Archived.vue'),
    meta: { nav: 'providers', crumb: 'providerArchive', crumbOnly: true },
  },
  { path: '/logs', component: () => import('../components/Logs/Index.vue'), meta: { nav: 'logs' } },
  { path: '/logs/session', component: () => import('../components/Logs/SessionTrace.vue'), meta: { nav: 'logs' } },
  { path: '/sessions', component: () => import('../components/Session/Index.vue'), meta: { nav: 'sessions' } },
  { path: '/mcp', component: () => import('../components/Mcp/Index.vue'), meta: { nav: 'mcp' } },
  {
    path: '/mcp/diagnostics',
    component: () => import('../components/Mcp/Diagnostics.vue'),
    meta: { nav: 'mcp', crumb: 'mcpDiagnostics' },
  },
  { path: '/settings', component: () => import('../components/General/Index.vue'), meta: { nav: 'settings' } },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

export default createRouter({
  history: createWebHashHistory(), // Use createWebHashHistory for hash-based routing
  routes
})

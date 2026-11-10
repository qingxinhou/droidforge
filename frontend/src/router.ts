import { createRouter, createWebHashHistory } from 'vue-router'

const routes = [
  { path: '/', name: 'dashboard', component: () => import('./views/Dashboard.vue') },
  { path: '/terminal', name: 'terminal', component: () => import('./views/AdbTerminal.vue') },
  { path: '/reboot', name: 'reboot', component: () => import('./views/Reboot.vue') },
  { path: '/apps', name: 'apps', component: () => import('./views/Apps.vue') },
  { path: '/files', name: 'files', component: () => import('./views/Files.vue') },
  { path: '/flash', name: 'flash', component: () => import('./views/FlashImage.vue') },
  { path: '/lineflash', name: 'lineflash', component: () => import('./views/LineFlash.vue') },
  { path: '/extract', name: 'extract', component: () => import('./views/Extractor.vue') },
  { path: '/display', name: 'display', component: () => import('./views/DisplayControl.vue') },
  { path: '/sysui', name: 'sysui', component: () => import('./views/SystemUi.vue') },
  { path: '/battery', name: 'battery', component: () => import('./views/Battery.vue') },
  { path: '/performance', name: 'performance', component: () => import('./views/Performance.vue') },
  { path: '/partitions', name: 'partitions', component: () => import('./views/Partitions.vue') },
  { path: '/scripts', name: 'scripts', component: () => import('./views/Scripts.vue') },
  { path: '/root', name: 'root', component: () => import('./views/RootCenter.vue') },
  { path: '/settings', name: 'settings', component: () => import('./views/Settings.vue') },
]

export default createRouter({
  history: createWebHashHistory(),
  routes,
})

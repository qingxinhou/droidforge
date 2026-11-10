<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import Sidebar from './components/Sidebar.vue'
import TopBar from './components/TopBar.vue'
import LogConsole from './components/LogConsole.vue'
import Toasts from './components/Toasts.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'
import CommandPalette from './components/CommandPalette.vue'
import DisclaimerGate from './components/DisclaimerGate.vue'
import DisclaimerDialog from './components/DisclaimerDialog.vue'
import { init, state } from './store'
import { DISCLAIMER_VERSION, legalState } from './legal'

const router = useRouter()

onMounted(async () => {
  await init()
  checkDisclaimer()
  const start = state.env?.config.startPage
  if (start && start !== '/' && router.currentRoute.value.path === '/') {
    router.replace(start)
  }
})

/** 未同意当前版本的免责声明时，强制显示协议门。 */
function checkDisclaimer() {
  const accepted = state.env?.config.disclaimerVersion ?? 0
  if (accepted < DISCLAIMER_VERSION) {
    legalState.gateOpen = true
  }
}
</script>

<template>
  <div class="flex h-full w-full bg-ink-950">
    <Sidebar />
    <div class="flex min-w-0 flex-1 flex-col">
      <TopBar />
      <main class="min-h-0 flex-1 overflow-y-auto">
        <router-view v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </main>
      <LogConsole />
    </div>
    <Toasts />
    <CommandPalette />
    <ConfirmDialog />
    <DisclaimerDialog />
    <DisclaimerGate />
  </div>
</template>

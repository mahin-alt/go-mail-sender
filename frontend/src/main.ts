import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'

import App from './App.vue'
import router from './router'
import { AppPreset } from './theme/preset'
import { queryClient } from './queries/client'

import './styles/main.css'

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(VueQueryPlugin, { queryClient })
app.use(PrimeVue, {
  theme: {
    preset: AppPreset,
    options: {
      // Light theme only for the MVP: stop PrimeVue following the OS dark mode.
      darkModeSelector: false,
      cssLayer: { name: 'primevue', order: 'theme, base, primevue, components, utilities' },
    },
  },
})

app.mount('#app')

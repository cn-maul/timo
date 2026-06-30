import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'

import './styles/island.css'
import './styles/animations.css'
import './styles/themes/dark.css'
import './styles/themes/light.css'
import './styles/themes/frosted.css'

const app = createApp(App)
app.use(createPinia())
app.mount('#app')

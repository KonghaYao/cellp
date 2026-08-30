import DefaultTheme from 'vitepress/theme'
import type { Theme } from 'vitepress'
import Flow from './components/Flow.vue'
import CalloutGrid from './components/CalloutGrid.vue'
import CompareTable from './components/CompareTable.vue'
import './custom.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('Flow', Flow)
    app.component('CalloutGrid', CalloutGrid)
    app.component('CompareTable', CompareTable)
  },
} satisfies Theme

<script setup lang="ts">
import { computed } from 'vue'
import {
  BarElement,
  CategoryScale,
  Chart as ChartJS,
  LinearScale,
  Tooltip,
  type ChartOptions,
} from 'chart.js'
import { Bar } from 'vue-chartjs'

import { themeColor } from '@/lib/chartColors'
import { dayLabel, dayOfMonth } from '@/lib/dashboard'
import type { Stats } from '@/types/api'

ChartJS.register(BarElement, CategoryScale, LinearScale, Tooltip)

const props = defineProps<{ days: Stats['daily'] }>()

const ok = themeColor('--color-ok', '#1f9d63')
const bad = themeColor('--color-bad', '#e5484d')
const muted = themeColor('--color-muted', '#66645e')
const line = themeColor('--color-hairline', '#efeeea')

const data = computed(() => ({
  labels: props.days.map((d) => dayOfMonth(d.date)),
  datasets: [
    {
      label: 'Sent',
      data: props.days.map((d) => d.sent),
      backgroundColor: ok,
      borderRadius: 4,
      borderSkipped: false,
    },
    {
      label: 'Failed',
      data: props.days.map((d) => d.failed),
      backgroundColor: bad,
      borderRadius: 4,
      borderSkipped: false,
    },
  ],
}))

const options = computed<ChartOptions<'bar'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: { duration: 200 },
  interaction: { mode: 'index', intersect: false },
  scales: {
    x: { grid: { display: false }, ticks: { color: muted, font: { size: 10 }, maxRotation: 0 } },
    y: {
      beginAtZero: true,
      grid: { color: line },
      border: { display: false },
      ticks: { color: muted, font: { size: 10 }, precision: 0, maxTicksLimit: 4 },
    },
  },
  plugins: {
    legend: { display: false },
    tooltip: {
      callbacks: {
        title: (items) => (items[0] ? dayLabel(props.days[items[0].dataIndex]?.date ?? '') : ''),
        label: (item) => ` ${item.dataset.label}: ${Number(item.raw).toLocaleString('en')}`,
      },
    },
  },
}))

const summary = computed(() => {
  const sent = props.days.reduce((n, d) => n + d.sent, 0)
  const failed = props.days.reduce((n, d) => n + d.failed, 0)
  return `Emails per day for the last ${props.days.length} days: ${sent.toLocaleString('en')} sent and ${failed.toLocaleString('en')} failed in total. Use "Show as table" for each day.`
})
</script>

<template>
  <div class="h-56" role="img" :aria-label="summary">
    <Bar :data="data" :options="options" aria-hidden="true" />
  </div>
</template>

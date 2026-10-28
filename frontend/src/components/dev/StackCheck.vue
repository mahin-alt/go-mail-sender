<script setup lang="ts">
import Button from 'primevue/button'
import StatusBadge from '@/components/status/StatusBadge.vue'
import ProgressBar from 'primevue/progressbar'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'

// Dev-only page that proves the stack is wired: fonts (Latin + Bangla), PrimeVue theme,
// Tailwind utilities beating PrimeVue styles, and the tabular DataTable density.
const rows = [
  { email: 'rahim@example.com', name: 'রহিম উদ্দিন', status: 'sent', attempts: 1 },
  { email: 'karim@example.com', name: 'Karim Hossain', status: 'failed', attempts: 4 },
  { email: 'sumi@example.com', name: 'সুমি আক্তার', status: 'pending', attempts: 0 },
]
</script>

<template>
  <main class="mx-auto flex max-w-3xl flex-col gap-6">
    <h1 class="text-xl font-semibold">Stack check (dev only)</h1>

    <section class="flex flex-col gap-2 rounded-lg border border-surface-200 bg-surface-0 p-4">
      <h2 class="text-base font-semibold">Fonts</h2>
      <p data-testid="latin">Order #1042 has been sent to the merchant.</p>
      <p data-testid="bangla" lang="bn">আপনার অর্ডার #১০৪২ ব্যবসায়ীর কাছে পাঠানো হয়েছে।</p>
      <p data-testid="mixed" class="text-user">Dear রহিম উদ্দিন, your order is on the way.</p>
      <p class="tabular-nums">1,234 &nbsp; 98,765 &nbsp; 4 of 1,000 sent</p>
    </section>

    <section class="flex flex-col gap-3 rounded-lg border border-surface-200 bg-surface-0 p-4">
      <h2 class="text-base font-semibold">PrimeVue and Tailwind layering</h2>
      <div class="flex items-center gap-3">
        <Button data-testid="btn-default" label="Default primary" />
        <Button data-testid="btn-override" label="Tailwind override" class="bg-red-600" />
        <span class="rounded-md bg-primary px-2 py-1 text-primary-contrast">bg-primary token</span>
      </div>
      <ProgressBar :value="38" />
      <div class="flex flex-wrap gap-2">
        <StatusBadge status="sent" />
        <StatusBadge status="failed" />
        <StatusBadge status="invalid" />
        <StatusBadge status="pending" />
        <StatusBadge status="pending" :attempts="2" :max-attempts="4" />
        <StatusBadge status="processing" />
        <StatusBadge status="completed" />
      </div>
    </section>

    <section class="rounded-lg border border-surface-200 bg-surface-0 p-4">
      <DataTable :value="rows" size="small" data-key="email">
        <Column field="email" header="Email" />
        <Column field="name" header="Name">
          <template #body="{ data }"
            ><span lang="bn" class="text-user">{{ data.name }}</span></template
          >
        </Column>
        <Column field="status" header="Status" />
        <Column
          field="attempts"
          header="Attempts"
          body-class="text-right tabular-nums"
          header-class="text-right"
        />
      </DataTable>
    </section>
  </main>
</template>

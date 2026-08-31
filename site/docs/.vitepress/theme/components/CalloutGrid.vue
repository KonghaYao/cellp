<script setup lang="ts">
defineProps<{
  items: { title: string; body: string; badge?: string }[]
}>()
</script>

<template>
  <div class="cellp-grid">
    <div v-for="item in items" :key="item.title" class="cellp-card">
      <div v-if="item.badge" class="cellp-badge">{{ item.badge }}</div>
      <h3>{{ item.title }}</h3>
      <p>{{ item.body }}</p>
    </div>
  </div>
</template>

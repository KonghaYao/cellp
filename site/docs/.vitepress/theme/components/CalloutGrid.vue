<script setup lang="ts">
defineProps<{
  items: { title: string; body: string }[]
}>()
</script>

<template>
  <div class="cellp-grid">
    <div v-for="item in items" :key="item.title" class="cellp-card">
      <h3>{{ item.title }}</h3>
      <p>{{ item.body }}</p>
    </div>
  </div>
</template>

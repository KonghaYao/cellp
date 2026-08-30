<script setup lang="ts">
defineProps<{
  steps: string[]
}>()
</script>

<template>
  <div class="cellp-flow" role="list">
    <div v-for="(step, i) in steps" :key="i" class="step" role="listitem">
      <span class="n">{{ String(i + 1).padStart(2, '0') }}</span>
      <span>{{ step }}</span>
    </div>
  </div>
</template>

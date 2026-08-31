<script setup lang="ts">
import { withBase } from 'vitepress'

defineProps<{
  items: { name: string; href: string; forks?: boolean }[]
}>()
</script>

<template>
  <div class="cellp-binding-strip" role="list">
    <template v-for="(item, i) in items" :key="item.name">
      <span v-if="i > 0" class="plus" aria-hidden="true">+</span>
      <a class="pill" :class="{ forks: item.forks }" :href="withBase(item.href)" role="listitem">
        <span class="name">{{ item.name }}</span>
        <span v-if="item.forks" class="mark">branches</span>
      </a>
    </template>
  </div>
</template>

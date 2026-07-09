<template>
  <div class="model-capability-tags">
    <span
      v-for="capability in capabilities"
      :key="capability"
      class="model-capability-tag"
    >
      {{ t(`modelCatalog.capabilities.${capability}`) }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{
  capabilities: string[]
}>()

const { t } = useI18n()
</script>

<style scoped>
.model-capability-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.model-capability-tag {
  border: 1px solid rgba(94, 234, 212, 0.22);
  background: rgba(15, 23, 42, 0.52);
  color: #b9fff4;
  font-size: 11px;
  font-weight: 700;
  line-height: 1;
  padding: 7px 9px;
  text-transform: uppercase;
}
</style>

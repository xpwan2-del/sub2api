<template>
  <div class="model-price-summary">
    <div class="model-price-kicker">{{ billingLabel }}</div>
    <div v-if="pricing" class="model-price-grid">
      <div v-if="pricing.input_price != null" class="model-price-cell">
        <span>{{ t('modelCatalog.price.input') }}</span>
        <strong>{{ formatScaled(pricing.input_price, tokenScale) }}</strong>
      </div>
      <div v-if="pricing.output_price != null" class="model-price-cell">
        <span>{{ t('modelCatalog.price.output') }}</span>
        <strong>{{ formatScaled(pricing.output_price, tokenScale) }}</strong>
      </div>
      <div v-if="hasPositivePrice(pricing.image_output_price)" class="model-price-cell">
        <span>{{ t('modelCatalog.price.image') }}</span>
        <strong>{{ formatScaled(pricing.image_output_price, 1) }}</strong>
      </div>
      <div v-if="pricing.per_request_price != null" class="model-price-cell">
        <span>{{ t('modelCatalog.price.request') }}</span>
        <strong>{{ formatScaled(pricing.per_request_price, 1) }}</strong>
      </div>
      <div v-if="!hasVisiblePrice" class="model-price-empty">
        {{ t('modelCatalog.price.unavailable') }}
      </div>
    </div>
    <div v-else class="model-price-empty">
      {{ t('modelCatalog.price.unavailable') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatScaled } from '@/utils/pricing'
import type { PublicModelPricing } from '@/api/publicModels'

const props = defineProps<{
  pricing: PublicModelPricing | null
}>()

const { t } = useI18n()
const tokenScale = 1_000_000

const hasVisiblePrice = computed(() => {
  const pricing = props.pricing
  return !!pricing && [
    pricing.input_price,
    pricing.output_price,
    hasPositivePrice(pricing.image_output_price) ? pricing.image_output_price : null,
    pricing.per_request_price
  ].some((value) => value != null)
})

const knownBillingModes = new Set(['token', 'image', 'per_request', 'unknown'])

const billingLabel = computed(() => {
  const mode = props.pricing?.billing_mode || 'unknown'
  return knownBillingModes.has(mode) ? t(`modelCatalog.billingModes.${mode}`) : mode
})

function hasPositivePrice(value: number | null): value is number {
  return typeof value === 'number' && value > 0
}
</script>

<style scoped>
.model-price-summary {
  border-top: 1px solid rgba(125, 211, 252, 0.16);
  padding-top: 14px;
}

.model-price-kicker {
  color: #67e8f9;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 11px;
  letter-spacing: 0;
  margin-bottom: 10px;
  text-transform: uppercase;
}

.model-price-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 9px;
}

.model-price-cell {
  border: 1px solid rgba(94, 234, 212, 0.15);
  background: rgba(8, 47, 73, 0.35);
  padding: 10px;
}

.model-price-cell span {
  display: block;
  color: rgba(226, 232, 240, 0.62);
  font-size: 12px;
  margin-bottom: 5px;
}

.model-price-cell strong {
  color: #f8fafc;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 15px;
}

.model-price-empty {
  color: rgba(226, 232, 240, 0.62);
  font-size: 13px;
}
</style>

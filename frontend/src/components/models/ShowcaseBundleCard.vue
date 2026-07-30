<script lang="ts">
/**
 * 模型广场套餐展示卡（公开页专用，深色 HUD 风格）。
 *
 * 与 bundles/BundlePlanCard 区别：本卡是模型广场主视觉（深色切角 + mint 描边 +
 * 内发光 + monospace），用 scoped CSS + isDark prop（非 Tailwind dark:），
 * 不含 concurrency/rpm/精确额度（公开 DTO 无此数据）。
 * 仅展示 PublicBundlePlan 字段。featured 由父级据 pickFeaturedPlan 传入。
 *
 * 交互：只有「查看详情」按钮触发 click（跳 /bundles）；点卡片其它区域无反应。
 * 描述过长时 hover 显示完整内容气泡（纯 CSS popover）。
 */
</script>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { platformLabel } from '@/utils/platformColors'
import { platformDotColor } from '@/utils/showcaseBundle'
import type { PublicBundlePlan } from '@/api/publicBundles'

const props = defineProps<{
  plan: PublicBundlePlan
  featured?: boolean
  isDark?: boolean
}>()
const emit = defineEmits<{ (e: 'click', plan: PublicBundlePlan): void }>()

const { t } = useI18n()

const TIER_CODE: Record<string, string> = { starter: 'STARTER', pro: 'PRO', enterprise: 'ENTERPRISE' }
const tierCode = computed(() => TIER_CODE[props.plan.tier] ?? props.plan.tier.toUpperCase())
const hasDiscount = computed(() => props.plan.original_price > props.plan.price)

function onClick() {
  emit('click', props.plan)
}
</script>

<template>
  <article
    class="sbc-card"
    :class="{ 'is-featured': featured, 'is-dark': isDark }"
  >
    <span v-if="featured" class="sbc-featured-badge">{{ t('modelCatalog.bundleFeaturedTag') }}</span>

    <span class="sbc-tier">{{ tierCode }}</span>
    <h3 class="sbc-name">{{ plan.name }}</h3>
    <div v-if="plan.description" class="sbc-desc-wrap">
      <p class="sbc-desc">{{ plan.description }}</p>
      <span class="sbc-desc-popover" role="tooltip">{{ plan.description }}</span>
    </div>

    <div class="sbc-price">
      <span class="sbc-price-num">${{ plan.price }}</span>
      <span v-if="hasDiscount" class="sbc-price-strike">${{ plan.original_price }}</span>
      <span class="sbc-price-unit">/ {{ plan.validity_days }}{{ t('modelCatalog.bundleDays') }}</span>
    </div>

    <ul v-if="plan.features.length" class="sbc-feats">
      <li v-for="(f, i) in plan.features" :key="i">{{ f }}</li>
    </ul>

    <div v-if="plan.platforms.length" class="sbc-platforms">
      <span v-for="p in plan.platforms" :key="p" class="sbc-pchip">
        <span class="sbc-pdot" :style="{ background: platformDotColor(p) }" />
        {{ platformLabel(p) }}
      </span>
    </div>

    <button type="button" class="sbc-cta" @click="onClick">
      {{ t('modelCatalog.bundleViewDetails') }}
    </button>
  </article>
</template>

<style scoped>
.sbc-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 22px 20px 20px;
  min-height: 380px;
  color: #f8fafc;
  border: 1px solid rgba(94, 234, 212, 0.20);
  background:
    linear-gradient(145deg, rgba(15, 23, 42, 0.94), rgba(3, 7, 18, 0.86));
  box-shadow: 0 26px 60px rgba(0, 0, 0, 0.32), inset 0 0 32px rgba(45, 212, 191, 0.04);
  clip-path: polygon(0 0, calc(100% - 18px) 0, 100% 18px, 100% 100%, 18px 100%, 0 calc(100% - 18px));
}
.sbc-card::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: radial-gradient(circle at 20% 0%, rgba(34, 211, 238, 0.16), transparent 38%);
}
.sbc-card.is-featured {
  border-color: rgba(45, 212, 191, 0.45);
  box-shadow: 0 30px 70px rgba(0, 0, 0, 0.36), inset 0 0 40px rgba(45, 212, 191, 0.09);
}
.sbc-card.is-featured::before {
  background: radial-gradient(circle at 20% 0%, rgba(34, 211, 238, 0.26), transparent 42%);
}

.sbc-featured-badge {
  position: absolute;
  top: 0;
  right: 0;
  z-index: 2;
  padding: 5px 11px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: #022622;
  background: linear-gradient(135deg, rgba(52, 211, 153, 0.96), rgba(16, 185, 129, 0.88));
  box-shadow: 0 6px 16px rgba(16, 185, 129, 0.45);
  clip-path: polygon(0 0, 100% 0, 100% 100%, 14px 100%);
}

.sbc-tier {
  align-self: flex-start;
  padding: 3px 8px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: #b9fff4;
  background: rgba(45, 212, 191, 0.12);
  border: 1px solid rgba(94, 234, 212, 0.25);
  border-radius: 4px;
}
.sbc-name {
  margin: 2px 0 0;
  font-size: 19px;
  font-weight: 850;
  line-height: 1.15;
  color: #f8fafc;
}

/* 描述：两行截断 + hover 气泡显示全文 */
.sbc-desc-wrap { position: relative; cursor: help; }
.sbc-desc {
  margin: -6px 0 0;
  min-height: 32px;
  font-size: 12px;
  line-height: 1.5;
  color: rgba(203, 213, 225, 0.68);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.sbc-desc-popover {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  z-index: 20;
  width: max-content;
  max-width: 260px;
  padding: 10px 12px;
  font-size: 12px;
  line-height: 1.55;
  color: #f8fafc;
  background: rgba(2, 6, 23, 0.96);
  border: 1px solid rgba(94, 234, 212, 0.25);
  border-radius: 6px;
  box-shadow: 0 14px 32px rgba(0, 0, 0, 0.55);
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
  transition: opacity 0.15s;
}
.sbc-desc-wrap:hover .sbc-desc-popover {
  opacity: 1;
  visibility: visible;
}

.sbc-price {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 10px 0 4px;
  border-top: 1px solid rgba(94, 234, 212, 0.14);
}
.sbc-price-num {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 26px;
  font-weight: 800;
  line-height: 1;
  color: #67e8f9;
}
.sbc-price-strike {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
  color: rgba(203, 213, 225, 0.45);
  text-decoration: line-through;
}
.sbc-price-unit {
  margin-left: auto;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 11px;
  color: rgba(203, 213, 225, 0.6);
}

.sbc-feats {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.sbc-feats li {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  font-size: 12.5px;
  line-height: 1.4;
  color: rgba(226, 232, 240, 0.84);
}
.sbc-feats li::before {
  content: "";
  flex: none;
  width: 7px;
  height: 7px;
  margin-top: 5px;
  background: #2dd4bf;
  transform: rotate(45deg);
}

.sbc-platforms {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: auto;
  padding-top: 12px;
  border-top: 1px solid rgba(94, 234, 212, 0.14);
}
.sbc-pchip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 8px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 10px;
  font-weight: 600;
  color: #b9fff4;
  background: rgba(2, 6, 23, 0.5);
  border: 1px solid rgba(125, 211, 252, 0.22);
  border-radius: 4px;
}
.sbc-pdot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}

.sbc-cta {
  display: block;
  margin-top: 12px;
  padding: 11px 14px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  text-align: center;
  color: #b9fff4;
  background: transparent;
  border: 1.5px solid rgba(45, 212, 191, 0.5);
  cursor: pointer;
  transition: all 0.15s;
}
.sbc-cta:hover {
  color: #022622;
  background: linear-gradient(135deg, rgba(20, 184, 166, 0.96), rgba(34, 211, 238, 0.88));
  border-color: transparent;
}
/* featured 卡的 CTA 默认实心 */
.sbc-card.is-featured .sbc-cta {
  color: #022622;
  background: linear-gradient(135deg, rgba(20, 184, 166, 0.96), rgba(34, 211, 238, 0.88));
  border-color: transparent;
}
</style>

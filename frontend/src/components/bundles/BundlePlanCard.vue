<script lang="ts">
/**
 * 公共套餐卡组件 —— 单个套餐计划的展示外壳。
 *
 * 供两处复用：
 *  - `/bundles`（BundlesView）：通过 `#actions` 具名插槽注入购买/升级按钮，
 *    交互与原内联渲染完全一致。
 *  - 模型广场顶部套餐区（B3）：`showActions=false` 或监听 `@click` 跳转 `/bundles`。
 *
 * `plan` 同时兼容完整 `BundlePlan`（BundlesView）与裁剪后的 `PublicBundlePlan`（B3）；
 * BundlePlan 独有字段（group_quotas / concurrency_limit / rpm_limit）在此声明为可选，
 * 两种数据形态都能满足。所有可选展示区均以 `v-if` + 可选链守卫，缺失字段不会报错。
 *
 * 覆盖范围渲染采用同一视觉槽位的两种互斥形态：
 *  - group_quotas（BundlesView）：富 chips，含分组名 + 平台徽章 + 额度 tooltip。
 *  - platforms（B3 公开套餐）：扁平平台 chips（公开 DTO 不含精确额度）。
 * 优先渲染 group_quotas；缺省时回落到 platforms。BundlesView 不传 platforms（undefined），
 * 故其渲染路径完全不变。
 */
import type { BundlePlanGroupQuota } from '@/types/bundle'

/** BundlePlanCard 接受的套餐展示数据（BundlePlan 与 PublicBundlePlan 的共同展示字段） */
export interface BundlePlanCardData {
  id: number | string
  name: string
  description?: string
  tier?: string
  featured?: boolean
  price: number
  original_price?: number
  validity_days: number
  concurrency_limit?: number
  rpm_limit?: number
  features?: string[]
  group_quotas?: readonly BundlePlanGroupQuota[]
  /** 覆盖平台扁平列表（公开套餐 DTO 独有；BundlesView 的 BundlePlan 不含此字段） */
  platforms?: string[]
}
</script>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { platformBadgeLightClass, platformLabel } from '@/utils/platformColors'
import { getTierTheme, getTierI18nKey } from '@/constants/bundleTiers'
import { formatSegmentValue, hasAnyQuotaLimit, quotaSegments } from '@/utils/bundleQuota'

withDefaults(defineProps<{
  plan: BundlePlanCardData
  /** 是否渲染底部操作区（购买/升级按钮插槽）。B3 广场跳转模式可置 false。 */
  showActions?: boolean
}>(), {
  showActions: true,
})

// 点击整张卡（B3 广场用于跳转 /bundles）；BundlesView 不监听，无副作用。
const emit = defineEmits<{
  (e: 'click', plan: BundlePlanCardData): void
}>()

const { t } = useI18n()

// ── 层级主题工具（徽章/边框/强调色等），未知 tier 回退默认主题 ──
function tierLabel(tier?: string): string {
  return t(getTierI18nKey(tier, 'user'))
}

function tierBadgeClass(tier?: string): string {
  return getTierTheme(tier).badgeClass
}

function tierBorderClass(tier?: string): string {
  return getTierTheme(tier).borderClass
}

function tierAccentClass(tier?: string): string {
  return getTierTheme(tier).accentClass
}

function tierTextClass(tier?: string): string {
  return getTierTheme(tier).textClass
}

function tierIconClass(tier?: string): string {
  return getTierTheme(tier).iconClass
}

function tierDiscountClass(tier?: string): string {
  return getTierTheme(tier).discountClass
}

// 平台标识点颜色（openai=绿/anthropic=橙/antigravity=紫/gemini=蓝）
// NOTE: 与 BundlesView / PaymentView / BundleUsageView 内同名本地函数保持一致；
// 平台点色目前在各视图各自定义，后续可统一下沉到 platformColors。
function platformDotClass(p: string): string {
  switch (p) {
    case 'anthropic': return 'bg-orange-500'
    case 'openai': return 'bg-emerald-500'
    case 'antigravity': return 'bg-purple-500'
    case 'gemini': return 'bg-blue-500'
    default: return 'bg-gray-400'
  }
}
</script>

<template>
  <div
    class="group relative flex flex-col overflow-hidden rounded-2xl border transition-all hover:shadow-xl hover:-translate-y-0.5 bg-white dark:bg-dark-800"
    :class="tierBorderClass(plan.tier)"
    @click="emit('click', plan)"
  >
    <!-- Tier accent bar -->
    <div :class="['h-1.5', tierAccentClass(plan.tier)]" />

    <div class="flex flex-1 flex-col p-4">
      <!-- Header -->
      <div class="mb-3 flex items-start justify-between gap-2">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <h3 class="truncate text-base font-bold text-gray-900 dark:text-white">{{ plan.name }}</h3>
            <span :class="tierBadgeClass(plan.tier)">{{ tierLabel(plan.tier) }}</span>
            <span v-if="plan.featured" class="rounded-full border border-primary-500/30 bg-primary-500/10 px-2 py-0.5 text-[11px] font-medium text-primary-600 dark:text-primary-400">{{ t('modelCatalog.bundleFeaturedTag') }}</span>
          </div>
          <p v-if="plan.description"
            class="mt-0.5 line-clamp-2 text-xs leading-relaxed text-gray-500 dark:text-dark-400"
            :title="plan.description">
            {{ plan.description }}
          </p>
        </div>
        <div class="shrink-0 text-right">
          <div class="flex items-baseline gap-1">
            <span class="text-xs text-gray-400 dark:text-dark-500">$</span>
            <span :class="['text-2xl font-extrabold tracking-tight', tierTextClass(plan.tier)]">{{ plan.price }}</span>
          </div>
          <span class="text-[11px] text-gray-400 dark:text-dark-500">/ {{ plan.validity_days }}{{ t('bundles.days') }}</span>
          <div v-if="plan.original_price && plan.original_price > plan.price" class="mt-0.5 flex items-center justify-end gap-1.5">
            <span class="text-xs text-gray-400 line-through dark:text-dark-500">${{ plan.original_price }}</span>
            <span :class="['rounded px-1 py-0.5 text-[10px] font-semibold', tierDiscountClass(plan.tier)]">
              -{{ Math.round((1 - plan.price / plan.original_price) * 100) }}%
            </span>
          </div>
        </div>
      </div>

      <!-- Features (hero section) -->
      <div v-if="plan.features?.length" class="mb-3 rounded-lg bg-gray-50 px-3 py-2.5 dark:bg-dark-700/50">
        <div class="space-y-1.5">
          <div v-for="feature in plan.features" :key="feature" class="flex items-start gap-2">
            <svg :class="['mt-0.5 h-4 w-4 flex-shrink-0', tierIconClass(plan.tier)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
            </svg>
            <span class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ feature }}</span>
          </div>
        </div>
      </div>

      <!-- Group Quotas (summary + compact chips) -->
      <div v-if="plan.group_quotas?.length" class="mb-3">
        <p class="mb-1.5 text-xs font-medium text-gray-500 dark:text-gray-400">
          {{ t('bundles.includesGroupCount', { count: plan.group_quotas.length }) }}
        </p>
        <div class="flex flex-wrap gap-1.5">
          <div
            v-for="gq in plan.group_quotas"
            :key="gq.id"
            class="group/Chip relative flex items-center gap-1.5 rounded-lg border border-gray-100 bg-white px-2.5 py-1.5 dark:border-dark-600 dark:bg-dark-800"
          >
            <span :class="['h-1.5 w-1.5 rounded-full', platformDotClass(gq.group_platform || '')]" />
            <span class="text-[11px] font-medium text-gray-700 dark:text-gray-300">{{ gq.group_name || `Group #${gq.group_id}` }}</span>
            <span :class="['rounded px-1 py-0.5 text-[10px] font-medium', platformBadgeLightClass(gq.group_platform || '')]">
              {{ platformLabel(gq.group_platform || '') }}
            </span>
            <!-- Hover tooltip with quota details -->
            <div v-if="hasAnyQuotaLimit(gq)"
              class="pointer-events-none absolute left-1/2 top-full z-10 mt-1 -translate-x-1/2 rounded-lg border border-gray-200 bg-white px-3 py-2 opacity-0 shadow-lg transition-opacity group-hover/Chip:opacity-100 dark:border-dark-600 dark:bg-dark-800">
              <div class="flex gap-3 whitespace-nowrap text-[11px]">
                <template v-for="seg in quotaSegments(gq)" :key="seg.kind">
                  <span v-if="seg.daily"><span class="text-gray-400 dark:text-dark-500">{{ t('bundles.daily') }} </span><span class="font-medium text-gray-700 dark:text-gray-300">{{ formatSegmentValue(seg.kind, seg.daily) }}</span></span>
                  <span v-if="seg.weekly"><span class="text-gray-400 dark:text-dark-500">{{ t('bundles.weekly') }} </span><span class="font-medium text-gray-700 dark:text-gray-300">{{ formatSegmentValue(seg.kind, seg.weekly) }}</span></span>
                  <span v-if="seg.monthly"><span class="text-gray-400 dark:text-dark-500">{{ t('bundles.monthly') }} </span><span class="font-medium text-gray-700 dark:text-gray-300">{{ formatSegmentValue(seg.kind, seg.monthly) }}</span></span>
                </template>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Platforms (flat chips; 公开套餐仅含平台聚合，无 group_quotas 时回落渲染) -->
      <div v-else-if="plan.platforms?.length" class="mb-3">
        <p class="mb-1.5 text-xs font-medium text-gray-500 dark:text-gray-400">
          {{ t('bundles.coveredPlatforms', { count: plan.platforms.length }) }}
        </p>
        <div class="flex flex-wrap gap-1.5">
          <div
            v-for="p in plan.platforms"
            :key="p"
            class="flex items-center gap-1.5 rounded-lg border border-gray-100 bg-white px-2.5 py-1.5 dark:border-dark-600 dark:bg-dark-800"
          >
            <span :class="['h-1.5 w-1.5 rounded-full', platformDotClass(p)]" />
            <span :class="['rounded px-1 py-0.5 text-[10px] font-medium', platformBadgeLightClass(p)]">
              {{ platformLabel(p) }}
            </span>
          </div>
        </div>
      </div>

      <!-- Concurrency / RPM -->
      <div class="mb-3 flex gap-3 text-xs">
        <div v-if="plan.concurrency_limit" class="flex items-center gap-1 text-gray-500 dark:text-gray-400">
          <Icon name="bolt" size="xs" />
          <span>{{ plan.concurrency_limit }} {{ t('bundles.concurrencyShort') }}</span>
        </div>
        <div v-if="plan.rpm_limit" class="flex items-center gap-1 text-gray-500 dark:text-gray-400">
          <Icon name="clock" size="xs" />
          <span>{{ plan.rpm_limit }} RPM</span>
        </div>
      </div>

      <div class="flex-1" />

      <!-- 操作区（购买/升级按钮）由父级经 #actions 插槽注入；showActions 控制显隐 -->
      <template v-if="showActions && $slots.actions">
        <slot name="actions" />
      </template>
    </div>
  </div>
</template>

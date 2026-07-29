<script lang="ts">
/**
 * 模型广场顶部套餐展示区。
 *
 * 复用 BundlePlanCard（showActions=false）渲染公开套餐卡片列表；
 * 点击任意套餐卡（或「查看全部」CTA）跳转 /bundles —— 未登录由该路由的
 * requiresAuth 拦截 → 登录后回跳到 /bundles。
 *
 * 数据来自公开接口 GET /public/bundles/plans（PublicBundlePlan，裁剪版）。
 * 后端运营总开关 ops_enabled=false 时返回 []，父级据 bundlePlans.length v-if
 * 不渲染整区，故本组件无需感知开关。空数据时不渲染（由父级控制）。
 *
 * PublicBundlePlan 与 BundlePlanCardData 的差异：无 id（用 sort_order 作合成 id）、
 * 无 group_quotas / concurrency / rpm（卡片对应区块不渲染）；多出 platforms
 * （覆盖范围槽位回落渲染为扁平平台 chips）。
 *
 * 暗色：useThemeMode 切换 html 的 .dark 类，故沿用 Tailwind dark: 变体，
 * 与 BundlePlanCard / 模型广场其余组件一致。
 */
import type { PublicBundlePlan } from '@/api/publicBundles'
import type { BundlePlanCardData } from '@/components/bundles/BundlePlanCard.vue'

/** 把公开套餐 DTO 映射为卡片展示数据。 */
function toCardData(plan: PublicBundlePlan): BundlePlanCardData {
  return {
    id: plan.sort_order,
    name: plan.name,
    description: plan.description,
    tier: plan.tier,
    price: plan.price,
    original_price: plan.original_price,
    validity_days: plan.validity_days,
    features: plan.features,
    platforms: plan.platforms,
    // group_quotas / concurrency_limit / rpm_limit: 公开 DTO 不含 → 留空，区块不渲染
  }
}
</script>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import BundlePlanCard from '@/components/bundles/BundlePlanCard.vue'

defineProps<{
  plans: PublicBundlePlan[]
}>()

const router = useRouter()
const { t } = useI18n()

function goToBundles() {
  router.push('/bundles')
}
</script>

<template>
  <section class="bundle-showcase">
    <div class="flex items-end justify-between gap-4 sm:flex-row sm:items-end flex-col items-start">
      <div class="min-w-0">
        <h2 class="text-lg font-extrabold tracking-tight text-gray-900 dark:text-white sm:text-xl md:text-2xl">
          {{ t('modelCatalog.bundleSectionTitle') }}
        </h2>
        <p class="mt-1 text-[13px] text-slate-500 dark:text-slate-400">
          {{ t('modelCatalog.bundleSectionSubtitle') }}
        </p>
      </div>
      <button
        type="button"
        class="bundle-showcase-cta inline-flex flex-shrink-0 items-center gap-1 rounded-[10px] bg-gradient-to-br from-teal-500 to-teal-600 px-3.5 py-2 text-[13px] font-bold text-white shadow-[0_6px_18px_-6px_rgba(20,184,166,0.6)] transition-all hover:-translate-y-0.5 hover:shadow-[0_10px_22px_-6px_rgba(20,184,166,0.7)] active:translate-y-0 active:opacity-90"
        @click="goToBundles"
      >
        <span>{{ t('modelCatalog.viewAllBundles') }}</span>
        <svg class="h-4 w-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
        </svg>
      </button>
    </div>

    <div class="bundle-showcase-grid mt-4">
      <BundlePlanCard
        v-for="plan in plans"
        :key="`${plan.sort_order}-${plan.name}`"
        :plan="toCardData(plan)"
        :show-actions="false"
        @click="goToBundles"
      />
    </div>
  </section>
</template>

<style scoped>
/* auto-fit 网格 Tailwind 无法等价表达，故用 scoped CSS：宽屏 3 列、中屏 2 列、窄屏 1 列 */
.bundle-showcase-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 18px;
}
</style>

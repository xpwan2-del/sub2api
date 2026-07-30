<script lang="ts">
/**
 * 模型广场顶部套餐展示区（公开页）。
 *
 * 改用专属 ShowcaseBundleCard（深色 HUD），不再复用 bundles/BundlePlanCard。
 * featured（主推）由 pickFeaturedPlan 判定，透传到对应卡。
 * 点击卡 / 「查看全部」均跳 /bundles（未登录由该路由 requiresAuth 拦截）。
 * isDark 由 ModelCatalogView 透传，再下传给每张卡（单套暗色机制）。
 */
</script>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ShowcaseBundleCard from '@/components/models/ShowcaseBundleCard.vue'
import { pickFeaturedPlan } from '@/utils/showcaseBundle'
import type { PublicBundlePlan } from '@/api/publicBundles'

const props = defineProps<{
  plans: PublicBundlePlan[]
  isDark?: boolean
}>()

const router = useRouter()
const { t } = useI18n()

const featuredPlan = computed(() => pickFeaturedPlan(props.plans))
const platformCount = computed(() =>
  new Set(props.plans.flatMap(p => p.platforms)).size,
)

function isFeatured(plan: PublicBundlePlan): boolean {
  const f = featuredPlan.value
  return !!f && f.sort_order === plan.sort_order && f.name === plan.name
}
function goToBundles() {
  router.push('/bundles')
}
</script>

<template>
  <section class="bundle-showcase" :class="{ 'is-dark': isDark }">
    <header class="bs-head">
      <div class="bs-head-text">
        <div class="bs-kicker">{{ t('modelCatalog.bundleSectionKicker') }}</div>
        <h1 class="bs-title">{{ t('modelCatalog.bundleSectionTitle') }}</h1>
        <p class="bs-sub">{{ t('modelCatalog.bundleSectionSubtitle') }}</p>
        <div class="bs-stats">
          {{ t('modelCatalog.bundleSectionStats', { n: plans.length, m: platformCount }) }}
        </div>
      </div>
      <button type="button" class="bs-viewall" @click="goToBundles">
        <span>{{ t('modelCatalog.viewAllBundles') }}</span>
        <svg class="bs-viewall-arrow" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
        </svg>
      </button>
    </header>

    <div class="bs-grid">
      <ShowcaseBundleCard
        v-for="plan in plans"
        :key="`${plan.sort_order}-${plan.name}`"
        :plan="plan"
        :featured="isFeatured(plan)"
        :is-dark="isDark"
        @click="goToBundles"
      />
    </div>
  </section>
</template>

<style scoped>
.bundle-showcase {
  --bs-fg: #0f172a;
  --bs-sub: #475569;
  --bs-teal: #14b8a6;
  color: var(--bs-fg);
}
.bundle-showcase.is-dark {
  --bs-fg: #f8fafc;
  --bs-sub: rgba(203, 213, 225, 0.78);
}

.bs-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 24px;
  margin-bottom: 22px;
}
.bs-head-text { min-width: 0; }
.bs-kicker {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
  font-weight: 800;
  text-transform: uppercase;
  color: #0f766e;
  margin: 0 0 14px;
}
.bs-title {
  margin: 0;
  font-size: clamp(32px, 5vw, 52px);
  font-weight: 950;
  line-height: 0.95;
}
.bs-sub {
  max-width: 560px;
  margin: 10px 0 0;
  font-size: 14px;
  line-height: 1.6;
  color: var(--bs-sub);
}
.bs-stats {
  margin-top: 8px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  font-weight: 700;
  color: #0d9488;
}
.bundle-showcase.is-dark .bs-stats { color: #2dd4bf; }
.bundle-showcase.is-dark .bs-kicker { color: #22d3ee; }

.bs-viewall {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 11px 18px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
  font-weight: 800;
  color: var(--bs-fg);
  background: rgba(255, 255, 255, 0.6);
  border: 1.5px solid rgba(20, 184, 166, 0.55);
  cursor: pointer;
  transition: all 0.15s;
}
.bundle-showcase.is-dark .bs-viewall {
  color: #f8fafc;
  background: rgba(2, 6, 23, 0.5);
}
.bs-viewall:hover {
  color: #022622;
  background: linear-gradient(135deg, rgba(20, 184, 166, 0.96), rgba(34, 211, 238, 0.88));
  border-color: transparent;
}
.bs-viewall-arrow { width: 16px; height: 16px; flex: none; }

.bs-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 18px;
}
</style>

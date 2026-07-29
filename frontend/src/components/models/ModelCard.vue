<template>
  <article class="model-catalog-card">
    <span v-if="model.pinned" class="model-card-pin">{{ t('modelCatalog.pinned') }}</span>
    <div class="model-card-header">
      <div class="model-card-mark">
        <PlatformIcon :platform="primaryPlatform" size="lg" />
      </div>
      <div class="model-card-title-block">
        <h3>{{ model.name }}</h3>
        <p>{{ model.provider }}</p>
      </div>
      <span class="model-card-status">{{ t('modelCatalog.available') }}</span>
    </div>

    <div v-if="opsBadges.length" class="model-card-ops-badges">
      <span
        v-for="badge in opsBadges"
        :key="badge.key"
        class="model-ops-badge"
        :class="`model-ops-badge--${badge.key}`"
      >{{ t(badge.label) }}</span>
    </div>

    <ModelCapabilityTags
      v-if="model.capabilities.length"
      :capabilities="model.capabilities"
    />

    <p class="model-card-description">{{ model.description }}</p>

    <div class="model-platform-row">
      <span
        v-for="platform in model.platforms"
        :key="platform"
        class="model-platform-chip"
      >
        {{ platform }}
      </span>
    </div>

    <ModelHealthBar :health="model.health" />

    <ModelPriceSummary :pricing="model.pricing" />

    <div class="model-card-actions">
      <button type="button" class="model-card-copy" @click="$emit('copy', model.name)">
        <Icon name="copy" size="sm" />
        <span>{{ t('modelCatalog.copyName') }}</span>
      </button>
      <router-link class="model-card-use" to="/register">
        <span>{{ t('modelCatalog.startUsing') }}</span>
        <Icon name="arrowRight" size="xs" />
      </router-link>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelCapabilityTags from './ModelCapabilityTags.vue'
import ModelHealthBar from './ModelHealthBar.vue'
import ModelPriceSummary from './ModelPriceSummary.vue'
import type { ModelCatalogCard } from '@/utils/modelCatalog'

const props = defineProps<{
  model: ModelCatalogCard
}>()

defineEmits<{
  copy: [name: string]
}>()

const { t } = useI18n()
const primaryPlatform = computed(() => props.model.platforms[0] as any)

interface OpsBadge {
  key: 'new' | 'featured' | 'recommended'
  label: string
}

// 运营 badge（强调色实心）与自动能力标签（ModelCapabilityTags 半透明描边 chip）刻意分层，
// 让用户一眼区分「运营强推」与「自动推断能力」。
const opsBadges = computed<OpsBadge[]>(() => {
  const badges: OpsBadge[] = []
  if (props.model.is_new) badges.push({ key: 'new', label: 'modelCatalog.badgeNew' })
  if (props.model.featured) badges.push({ key: 'featured', label: 'modelCatalog.badgeFeatured' })
  if (props.model.tags.includes('recommended')) {
    badges.push({ key: 'recommended', label: 'modelCatalog.badgeRecommended' })
  }
  return badges
})
</script>

<style scoped>
.model-catalog-card {
  position: relative;
  display: flex;
  min-height: 330px;
  flex-direction: column;
  gap: 18px;
  border: 1px solid rgba(94, 234, 212, 0.18);
  background:
    linear-gradient(145deg, rgba(15, 23, 42, 0.92), rgba(3, 7, 18, 0.84)),
    rgba(8, 13, 27, 0.88);
  box-shadow: 0 26px 70px rgba(0, 0, 0, 0.34), inset 0 0 32px rgba(45, 212, 191, 0.05);
  clip-path: polygon(0 0, calc(100% - 18px) 0, 100% 18px, 100% 100%, 18px 100%, 0 calc(100% - 18px));
  padding: 20px;
}

.model-catalog-card::before {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: radial-gradient(circle at 20% 0%, rgba(34, 211, 238, 0.16), transparent 36%);
  content: '';
}

.model-card-header {
  position: relative;
  display: flex;
  align-items: center;
  gap: 13px;
}

.model-card-mark {
  display: grid;
  width: 42px;
  height: 42px;
  place-items: center;
  border: 1px solid rgba(45, 212, 191, 0.35);
  background: rgba(14, 116, 144, 0.2);
  color: #99f6e4;
  box-shadow: 0 0 24px rgba(34, 211, 238, 0.15);
}

.model-card-title-block {
  min-width: 0;
  flex: 1;
}

.model-card-title-block h3 {
  overflow-wrap: anywhere;
  color: #f8fafc;
  font-size: 19px;
  font-weight: 850;
  line-height: 1.15;
  margin: 0;
}

.model-card-title-block p {
  color: rgba(203, 213, 225, 0.68);
  font-size: 13px;
  margin: 5px 0 0;
}

.model-card-status {
  border: 1px solid rgba(52, 211, 153, 0.28);
  color: #86efac;
  font-size: 11px;
  font-weight: 800;
  padding: 6px 8px;
  text-transform: uppercase;
}

/* 置顶角标：贴在卡片左上实体角（卡片右上 / 左下为切角透明区） */
.model-card-pin {
  position: absolute;
  top: 0;
  left: 0;
  z-index: 2;
  display: inline-flex;
  align-items: center;
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.06em;
  line-height: 1;
  padding: 6px 11px 6px 12px;
  text-transform: uppercase;
  background: linear-gradient(135deg, rgba(20, 184, 166, 0.96), rgba(34, 211, 238, 0.88));
  color: #022622;
  clip-path: polygon(0 0, 100% 0, 100% 100%, 9px 100%);
}

/* 运营 badge：实心饱和渐变，与半透明描边的能力 chip 形成层级区分 */
.model-card-ops-badges {
  position: relative;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.model-ops-badge {
  display: inline-flex;
  align-items: center;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.04em;
  line-height: 1;
  padding: 7px 9px;
  text-transform: uppercase;
}

.model-ops-badge--new {
  background: linear-gradient(135deg, rgba(252, 211, 77, 0.96), rgba(245, 158, 11, 0.88));
  color: #2a1605;
}

.model-ops-badge--featured {
  background: linear-gradient(135deg, rgba(52, 211, 153, 0.96), rgba(16, 185, 129, 0.88));
  color: #022c1e;
}

.model-ops-badge--recommended {
  background: linear-gradient(135deg, rgba(129, 140, 248, 0.96), rgba(99, 102, 241, 0.88));
  color: #0b1024;
}

.model-card-description {
  position: relative;
  min-height: 44px;
  color: rgba(203, 213, 225, 0.78);
  font-size: 13px;
  line-height: 1.65;
  margin: -3px 0 0;
}

.model-platform-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  min-height: 28px;
}

.model-platform-chip {
  border: 1px solid rgba(125, 211, 252, 0.18);
  color: rgba(226, 232, 240, 0.78);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 11px;
  padding: 6px 8px;
}

.model-card-actions {
  display: flex;
  gap: 10px;
  margin-top: auto;
}

.model-card-copy,
.model-card-use {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 38px;
  border: 1px solid rgba(94, 234, 212, 0.22);
  color: #dffefa;
  font-size: 13px;
  font-weight: 800;
  padding: 0 13px;
  text-decoration: none;
}

.model-card-copy {
  background: rgba(15, 23, 42, 0.5);
}

.model-card-use {
  background: linear-gradient(135deg, rgba(20, 184, 166, 0.28), rgba(59, 130, 246, 0.24));
}

@media (max-width: 520px) {
  .model-card-actions {
    flex-direction: column;
  }
}
</style>

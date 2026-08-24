<template>
  <div class="space-y-2">
    <p class="px-0.5 text-[11px] font-medium text-gray-500 dark:text-dark-400">
      {{ title }}
    </p>

    <!-- Guide not configured: generic hint -->
    <div
      v-if="commands.length === 0"
      class="flex items-center gap-2 rounded-lg border border-blue-200 bg-blue-50 p-2 dark:border-blue-800/50 dark:bg-blue-900/20"
    >
      <svg
        class="h-3.5 w-3.5 flex-shrink-0 text-blue-500 dark:text-blue-400"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
        />
      </svg>
      <p class="min-w-0 flex-1 text-xs leading-4 text-blue-600 dark:text-blue-400">
        {{ fallbackHint }}
      </p>
    </div>

    <OpsCommandBlock v-else :commands="commands" :note="note" />
  </div>
</template>

<script setup lang="ts">
import OpsCommandBlock from '@/components/common/OpsCommandBlock.vue'

defineProps<{
  /** 指引标题（后端 guide.title，缺省时调用方传 i18n 兜底） */
  title: string
  /** 部署体系注入的运维命令（空 = 指引未配置，渲染通用提示） */
  commands: string[]
  /** 附加警示（透传给 OpsCommandBlock） */
  note?: string
  /** 指引未配置时的通用提示文案 */
  fallbackHint: string
}>()
</script>

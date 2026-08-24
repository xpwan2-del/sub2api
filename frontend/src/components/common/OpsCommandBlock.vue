<template>
  <div class="space-y-2">
    <!-- Ops commands, one terminal-style block each -->
    <div
      v-for="(cmd, i) in commands"
      :key="i"
      class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600"
    >
      <div
        class="flex items-center justify-between border-b border-gray-200 bg-gray-100 px-2 py-1 dark:border-dark-600 dark:bg-dark-700"
      >
        <span class="select-none font-mono text-[10px] font-semibold text-gray-400 dark:text-dark-400"
          >$</span
        >
        <button
          @click="copyCommand(cmd)"
          class="flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-gray-400 transition-colors hover:bg-gray-200 hover:text-gray-600 dark:text-dark-400 dark:hover:bg-dark-600 dark:hover:text-dark-200"
        >
          <Icon
            :name="copiedCommand === cmd ? 'check' : 'copy'"
            size="xs"
            :stroke-width="2"
            :class="copiedCommand === cmd ? 'text-green-500' : ''"
          />
          {{ copiedCommand === cmd ? t('version.copied') : t('version.copyCommand') }}
        </button>
      </div>
      <code
        class="block select-all whitespace-pre-wrap break-all bg-gray-50 p-2.5 font-mono text-[10px] leading-relaxed text-gray-600 dark:bg-dark-900 dark:text-dark-300"
        >{{ cmd }}</code
      >
    </div>

    <!-- Ops note (e.g. "rollback only reverts the image, not the database") -->
    <p
      v-if="note"
      class="flex items-start gap-1.5 px-0.5 text-[11px] leading-4 text-amber-600 dark:text-amber-400"
    >
      <Icon name="exclamationTriangle" size="xs" :stroke-width="2" class="mt-px flex-shrink-0" />
      {{ note }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  /** 部署体系注入的运维命令（逐条渲染为终端风格块） */
  commands: string[]
  /** 附加警示（如"降级只回退镜像, 不回滚数据库"） */
  note?: string
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

// 逐条独立的复制状态（复制后短暂显示 ✓）
const copiedCommand = ref('')

async function copyCommand(cmd: string) {
  await copyToClipboard(cmd)
  copiedCommand.value = cmd
  setTimeout(() => {
    if (copiedCommand.value === cmd) copiedCommand.value = ''
  }, 2000)
}
</script>

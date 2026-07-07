<template>
  <div
    class="flex flex-wrap items-center gap-1 rounded-lg border border-gray-300 bg-white px-2 py-1 min-h-[2.5rem] dark:border-dark-600 dark:bg-dark-800"
    :class="{ 'pointer-events-none opacity-60 bg-gray-100 dark:bg-dark-700': disabled }"
  >
    <span
      v-for="(chip, idx) in chips"
      :key="`${chip}-${idx}`"
      class="chip inline-flex items-center gap-1 rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200"
    >
      {{ chip }}
      <button
        v-if="!disabled"
        type="button"
        class="chip-remove text-gray-400 hover:text-red-500"
        @click="removeChip(idx)"
      >×</button>
    </span>
    <input
      v-model="text"
      type="text"
      :disabled="disabled"
      :placeholder="chips.length === 0 ? placeholder : ''"
      class="tag-input min-w-[6rem] flex-1 border-none bg-transparent text-sm outline-none"
      @keydown="handleKeydown"
      @paste="handlePaste"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { splitModelPatterns } from '@/utils/bundleQuota'

const props = withDefaults(defineProps<{
  modelValue?: string
  placeholder?: string
  disabled?: boolean
}>(), {
  modelValue: '',
  placeholder: '',
  disabled: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const text = ref('')

// chips 由父级 modelValue 派生:单向数据流,编辑通过 emit 回写。
const chips = computed(() => splitModelPatterns(props.modelValue))

function emitChips(next: string[]) {
  emit('update:modelValue', next.join(','))
}

function commitText() {
  const parts = splitModelPatterns(text.value)
  if (parts.length === 0) return
  emitChips([...chips.value, ...parts])
  text.value = ''
}

function removeChip(idx: number) {
  emitChips(chips.value.filter((_, i) => i !== idx))
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' || e.key === ',') {
    e.preventDefault()
    commitText()
  } else if (e.key === 'Backspace' && text.value === '' && chips.value.length > 0) {
    removeChip(chips.value.length - 1)
  }
}

function handlePaste(e: ClipboardEvent) {
  const data = e.clipboardData?.getData('text') ?? ''
  if (!data.includes(',')) return // 无分隔符则走默认粘贴
  e.preventDefault()
  const parts = splitModelPatterns(data)
  if (parts.length > 0) emitChips([...chips.value, ...parts])
}
</script>

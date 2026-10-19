<template>
  <USelect
    :model-value="locale"
    :items="languages"
    value-key="value"
    label-key="label"
    icon="i-lucide-languages"
    :aria-label="t('language.label')"
    class="w-32 shrink-0"
    size="sm"
    @update:model-value="setLanguage"
  />
</template>

<script setup>
import { useI18n } from 'vue-i18n'
import { setLanguage } from '../i18n.js'

const { t, locale } = useI18n()
const languages = [
  { value: 'en', label: 'English' },
  { value: 'zh-CN', label: '简体中文' }
]
</script>

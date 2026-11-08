<template>
  <UModal :title="t('ui.font_missing_title')" :open="open" @update:open="val => emit('update:open', val)">
    <template #content>
      <div class="p-4 sm:p-6 space-y-4">
        <div class="flex items-start gap-3">
          <div class="p-2 bg-amber-500/10 text-amber-600 rounded-lg shrink-0 mt-0.5">
            <UIcon name="i-lucide-alert-triangle" class="w-6 h-6" />
          </div>
          <div class="min-w-0">
            <h3 class="text-base sm:text-lg font-semibold text-foreground">
              {{ t('ui.font_missing_title') }}
            </h3>
            <p class="text-sm text-muted mt-1 leading-relaxed">
              {{ t('ui.font_missing_desc') }}
            </p>
          </div>
        </div>

        <div v-if="missingFonts && missingFonts.length" class="space-y-1.5">
          <div class="text-xs font-medium text-muted uppercase tracking-wider">
            {{ t('ui.font_missing_list') }} ({{ missingFonts.length }})
          </div>
          <div class="max-h-36 overflow-y-auto rounded-md border border-default bg-elevated/50 p-2 space-y-1">
            <div
              v-for="font in missingFonts"
              :key="font"
              class="flex items-center gap-2 text-xs font-mono py-1 px-1.5 rounded hover:bg-muted/20"
            >
              <UIcon name="i-lucide-type" class="w-3.5 h-3.5 text-muted shrink-0" />
              <span class="truncate font-medium text-foreground">{{ font }}</span>
            </div>
          </div>
        </div>

        <div class="flex flex-col sm:flex-row sm:justify-end gap-2 pt-2 border-t border-default">
          <UButton
            variant="ghost"
            color="neutral"
            class="justify-center"
            @click="onCancel"
          >
            {{ t('ui.cancel') }}
          </UButton>
          <UButton
            variant="outline"
            color="warning"
            class="justify-center"
            @click="onProceedOnce"
          >
            {{ t('ui.allow_substitute_once') }}
          </UButton>
          <UButton
            color="primary"
            class="justify-center"
            @click="onProceedAlways"
          >
            {{ t('ui.always_allow_substitute') }}
          </UButton>
        </div>
      </div>
    </template>
  </UModal>
</template>

<script setup>
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps({
  open: { type: Boolean, default: false },
  missingFonts: { type: Array, default: () => [] }
})

const emit = defineEmits(['update:open', 'proceed', 'cancel'])

function onCancel() {
  emit('update:open', false)
  emit('cancel')
}

function onProceedOnce() {
  emit('update:open', false)
  emit('proceed', false)
}

function onProceedAlways() {
  emit('update:open', false)
  emit('proceed', true)
}
</script>

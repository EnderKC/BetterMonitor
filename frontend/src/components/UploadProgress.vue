<template>
  <div v-if="uploading" class="upload-progress">
    <div class="upload-progress__header">
      <span class="upload-progress__filename" :title="fileName">{{ fileName }}</span>
      <a-button type="link" danger size="small" @click="emit('cancel')">取消</a-button>
    </div>

    <a-progress :percent="progress" :status="progress >= 100 ? 'success' : 'active'" size="small" />

    <div class="upload-progress__meta">
      <span>{{ formatFileSize(uploadedSize) }} / {{ formatFileSize(totalSize) }}</span>
      <span v-if="totalChunks > 0" class="upload-progress__chunk">分片 {{ currentChunk }}/{{ totalChunks }}</span>
      <span>{{ formatSpeed(speed) }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { formatFileSize } from '../composables/useFileUpload'

interface Props {
  uploading: boolean
  progress: number
  fileName: string
  uploadedSize: number
  totalSize: number
  speed: number
  currentChunk?: number
  totalChunks?: number
}

withDefaults(defineProps<Props>(), {
  uploading: false,
  progress: 0,
  fileName: '',
  uploadedSize: 0,
  totalSize: 0,
  speed: 0,
  currentChunk: 0,
  totalChunks: 0,
})

const emit = defineEmits<{
  (event: 'cancel'): void
}>()

const formatSpeed = (bytesPerSecond: number): string => {
  if (bytesPerSecond <= 0) return '0 B/s'
  return `${formatFileSize(bytesPerSecond)}/s`
}
</script>

<style scoped>
.upload-progress {
  margin-top: 12px;
  padding: 12px;
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 8px;
  background: var(--bg-secondary, #fafafa);
}

.upload-progress__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.upload-progress__filename {
  max-width: calc(100% - 72px);
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  color: var(--text-primary, #111827);
  font-size: 13px;
}

.upload-progress__meta {
  margin-top: 6px;
  display: flex;
  justify-content: space-between;
  color: var(--text-secondary, #6b7280);
  font-size: 12px;
}

.upload-progress__chunk {
  color: var(--primary-color, #1890ff);
  font-weight: 500;
}
</style>

<style>
/* Upload progress dark mode overrides */
.dark .upload-progress {
  border-color: rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.04);
}

.dark .upload-progress__filename {
  color: #ccc;
}

.dark .upload-progress__meta {
  color: #999;
}
</style>

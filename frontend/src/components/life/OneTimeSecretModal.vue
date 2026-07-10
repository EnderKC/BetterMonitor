<script setup lang="ts">
import { message } from 'ant-design-vue';

defineProps<{
  visible: boolean;
  secret: string;
}>();

const emit = defineEmits<{
  close: [];
}>();

const copySecret = async (secret: string) => {
  try {
    await navigator.clipboard.writeText(secret);
    message.success('采集密钥已复制');
  } catch {
    message.error('复制失败，请手动复制');
  }
};
</script>

<template>
  <a-modal
    :open="visible"
    title="一次性采集密钥"
    :footer="null"
    :mask-closable="false"
    @cancel="emit('close')"
  >
    <p class="secret-warning">该密钥只显示本次，请立即保存到 LifeLogger 设备。</p>
    <div class="secret-value"><code>{{ secret }}</code></div>
    <div class="secret-actions">
      <button type="button" class="secret-button primary" data-testid="copy-secret" @click="copySecret(secret)">
        复制密钥
      </button>
      <button type="button" class="secret-button" data-testid="close-secret" @click="emit('close')">
        我已保存
      </button>
    </div>
  </a-modal>
</template>

<style scoped>
.secret-warning {
  color: #ad6800;
  line-height: 1.6;
}

.secret-value {
  padding: 14px;
  overflow-wrap: anywhere;
  border: 1px solid var(--border-color, #d9d9d9);
  border-radius: 8px;
  background: var(--code-bg, #f5f5f5);
}

.secret-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 20px;
}

.secret-button {
  padding: 6px 16px;
  cursor: pointer;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  background: #fff;
}

.secret-button.primary {
  color: #fff;
  border-color: #1677ff;
  background: #1677ff;
}
</style>

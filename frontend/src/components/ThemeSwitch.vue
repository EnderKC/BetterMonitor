<script setup lang="ts">
import { useThemeStore } from '@/stores/theme';
import { storeToRefs } from 'pinia';
import { BulbOutlined, BulbFilled, DesktopOutlined } from '@ant-design/icons-vue';

const themeStore = useThemeStore();
const { theme } = storeToRefs(themeStore);
</script>

<template>
  <div class="theme-switch">
    <a-segmented v-model:value="theme" :options="[
      { value: 'light', payload: { icon: BulbOutlined } },
      { value: 'dark', payload: { icon: BulbFilled } },
      { value: 'system', payload: { icon: DesktopOutlined } },
    ]" @change="themeStore.setTheme">
      <template #label="{ value, payload }">
        <div style="padding: 4px">
          <component :is="payload.icon" />
          <span v-if="false">{{ value }}</span>
        </div>
      </template>
    </a-segmented>
  </div>
</template>

<style scoped>
.theme-switch {
  display: inline-flex;
  align-items: center;
}
</style>

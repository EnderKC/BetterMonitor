<script setup lang="ts">
import { useThemeStore } from '@/stores/theme';
import { storeToRefs } from 'pinia';

const themeStore = useThemeStore();
const { isDark } = storeToRefs(themeStore);
</script>

<template>
    <div class="global-skeleton" :class="{ dark: isDark }">
        <!-- 模拟头部 -->
        <div class="skeleton-header">
            <a-skeleton-input active size="large" :style="{ width: '200px', height: '32px' }" />
            <div class="header-actions">
                <a-skeleton-button active size="default" shape="circle" />
                <a-skeleton-button active size="default" shape="round" :style="{ width: '100px' }" />
            </div>
        </div>

        <!-- 模拟卡片布局 -->
        <div class="skeleton-content">
            <!-- 横向统计卡片行 -->
            <div class="skeleton-row">
                <a-card class="skeleton-card" v-for="i in 3" :key="`stat-${i}`">
                    <a-skeleton active :paragraph="{ rows: 1 }" />
                </a-card>
            </div>

            <!-- 主要内容区 -->
            <a-card class="skeleton-card main-area">
                <div class="skeleton-list">
                    <div class="list-header">
                        <a-skeleton-input active size="default" :style="{ width: '150px' }" />
                    </div>
                    <div class="list-item" v-for="j in 5" :key="`list-${j}`">
                        <a-skeleton active avatar :paragraph="{ rows: 1 }" />
                    </div>
                </div>
            </a-card>
        </div>
    </div>
</template>

<style scoped>
.global-skeleton {
    width: 100%;
    height: 100%;
    padding: 0;
    box-sizing: border-box;
    animation: fadeIn 0.3s ease;
}

@keyframes fadeIn {
    from {
        opacity: 0;
    }

    to {
        opacity: 1;
    }
}

.skeleton-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 24px;
}

.header-actions {
    display: flex;
    gap: 16px;
}

.skeleton-row {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 24px;
    margin-bottom: 24px;
}

.skeleton-card {
    border-radius: 12px;
    background: rgba(255, 255, 255, 0.4);
    border: 1px solid rgba(255, 255, 255, 0.4);
}

.dark .skeleton-card {
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid rgba(255, 255, 255, 0.05);
}

.main-area {
    min-height: 400px;
}

.skeleton-list {
    display: flex;
    flex-direction: column;
    gap: 16px;
}

.list-header {
    margin-bottom: 16px;
    border-bottom: 1px solid rgba(0, 0, 0, 0.05);
    padding-bottom: 16px;
}

.dark .list-header {
    border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

@media (max-width: 768px) {
    .skeleton-row {
        grid-template-columns: 1fr;
    }
}
</style>

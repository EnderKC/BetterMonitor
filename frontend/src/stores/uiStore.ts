import { defineStore } from 'pinia';
import { ref } from 'vue';

export const useUIStore = defineStore('ui', () => {
    const isPageLoading = ref(false);

    // 最小显示时间，防止闪烁
    const MIN_LOADING_TIME = 300;
    let loadingStartTime = 0;
    let loadingTimer: any = null;

    const startLoading = () => {
        // 清除之前的定时器，防止冲突
        if (loadingTimer) {
            clearTimeout(loadingTimer);
            loadingTimer = null;
        }

        loadingStartTime = Date.now();
        isPageLoading.value = true;
    };

    const stopLoading = () => {
        const now = Date.now();
        const elapsed = now - loadingStartTime;
        const remaining = MIN_LOADING_TIME - elapsed;

        if (remaining > 0) {
            // 如果加载时间太短，延迟关闭
            loadingTimer = setTimeout(() => {
                isPageLoading.value = false;
                loadingTimer = null;
            }, remaining);
        } else {
            isPageLoading.value = false;
        }
    };

    return {
        isPageLoading,
        startLoading,
        stopLoading,
    };
});

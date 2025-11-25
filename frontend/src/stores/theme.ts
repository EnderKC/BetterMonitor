import { defineStore } from 'pinia';
import { ref, watch, onMounted } from 'vue';

export type Theme = 'light' | 'dark' | 'system';

export const useThemeStore = defineStore('theme', () => {
    const theme = ref<Theme>('system');
    const isDark = ref(false);

    // Initialize theme from local storage or default to system
    const initTheme = () => {
        const savedTheme = localStorage.getItem('theme') as Theme;
        if (savedTheme) {
            theme.value = savedTheme;
        }
        applyTheme();
    };

    // Set theme and save to local storage
    const setTheme = (newTheme: Theme) => {
        theme.value = newTheme;
        localStorage.setItem('theme', newTheme);
        applyTheme();
    };

    const syncDocumentBackground = () => {
        const root = document.documentElement;
        const bodyBg = getComputedStyle(root).getPropertyValue('--body-bg').trim();
        if (bodyBg) {
            root.style.backgroundColor = bodyBg;
            document.body.style.backgroundColor = bodyBg;
            const app = document.getElementById('app');
            if (app) {
                app.style.backgroundColor = bodyBg;
            }
        }
        root.style.colorScheme = isDark.value ? 'dark' : 'light';
    };

    // Apply theme logic
    const applyTheme = () => {
        if (theme.value === 'system') {
            const systemDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
            isDark.value = systemDark;
        } else {
            isDark.value = theme.value === 'dark';
        }

        if (isDark.value) {
            document.documentElement.classList.add('dark');
        } else {
            document.documentElement.classList.remove('dark');
        }

        syncDocumentBackground();
    };

    // Listen for system theme changes
    onMounted(() => {
        const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
        mediaQuery.addEventListener('change', (e) => {
            if (theme.value === 'system') {
                isDark.value = e.matches;
                applyTheme();
            }
        });
        initTheme();
    });

    return {
        theme,
        isDark,
        setTheme,
        initTheme,
    };
});

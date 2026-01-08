<template>
  <div class="code-editor-wrapper">
    <Codemirror
      v-model="internalValue"
      :extensions="extensions"
      :style="{ height: '100%', width: '100%' }"
      :autofocus="true"
      :indent-with-tab="true"
      :tab-size="2"
      @change="handleChange"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { Codemirror } from 'vue-codemirror';
import { basicSetup } from 'codemirror';
import { vscodeDark } from '@uiw/codemirror-theme-vscode';
import { javascript } from '@codemirror/lang-javascript';
import { html } from '@codemirror/lang-html';
import { css } from '@codemirror/lang-css';
import { json } from '@codemirror/lang-json';
import { python } from '@codemirror/lang-python';
import { markdown } from '@codemirror/lang-markdown';
import { xml } from '@codemirror/lang-xml';
import { sql } from '@codemirror/lang-sql';
import { java } from '@codemirror/lang-java';
import { cpp } from '@codemirror/lang-cpp';
import { php } from '@codemirror/lang-php';
import { rust } from '@codemirror/lang-rust';

const props = defineProps<{
  value: string;
  language?: string;
  filename?: string;
}>();

const emit = defineEmits<{
  (e: 'update:value', value: string): void;
  (e: 'change', value: string): void;
}>();

const internalValue = ref(props.value);

watch(() => props.value, (newVal) => {
  if (newVal !== internalValue.value) {
    internalValue.value = newVal;
  }
});

const detectLanguage = (filename: string) => {
  const ext = filename.split('.').pop()?.toLowerCase();
  switch (ext) {
    case 'js':
    case 'ts':
    case 'jsx':
    case 'tsx':
      return javascript();
    case 'html':
    case 'vue':
      return html();
    case 'css':
    case 'scss':
    case 'less':
      return css();
    case 'json':
      return json();
    case 'py':
      return python();
    case 'md':
      return markdown();
    case 'xml':
      return xml();
    case 'sql':
      return sql();
    case 'java':
      return java();
    case 'c':
    case 'cpp':
    case 'h':
      return cpp();
    case 'php':
      return php();
    case 'rs':
      return rust();
    default:
      return null;
  }
};

const extensions = computed(() => {
  const exts = [basicSetup, vscodeDark];
  
  if (props.language) {
    // If language is explicitly provided
    // (This part would need a mapping if language is a string)
  } else if (props.filename) {
    const langExt = detectLanguage(props.filename);
    if (langExt) exts.push(langExt);
  }
  
  return exts;
});

const handleChange = (val: string) => {
  emit('update:value', val);
  emit('change', val);
};
</script>

<style scoped>
.code-editor-wrapper {
  height: 100%;
  width: 100%;
  overflow: hidden;
  border-radius: 8px;
}
</style>

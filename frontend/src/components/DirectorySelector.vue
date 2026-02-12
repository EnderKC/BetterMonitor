<template>
  <div class="directory-selector">
    <a-modal
      v-model:visible="visible"
      title="选择目录"
      width="600px"
      @ok="handleOk"
      @cancel="handleCancel"
      :maskClosable="false"
    >
      <a-spin :spinning="loading">
        <div class="directory-browser">
          <!-- 当前路径显示 -->
          <div class="current-path">
            <a-breadcrumb>
              <!-- 根目录 -->
              <a-breadcrumb-item>
                <a @click="navigateToPath('/')">根目录</a>
              </a-breadcrumb-item>
              <!-- 其他路径部分 -->
              <a-breadcrumb-item v-for="(part, index) in pathParts" :key="index">
                <a @click="navigateToPath(getPathFromIndex(index))">{{ part }}</a>
              </a-breadcrumb-item>
            </a-breadcrumb>
          </div>
          
          <!-- 搜索框 -->
          <div class="search-box">
            <a-input
              v-model:value="searchKeyword"
              placeholder="搜索当前目录中的文件夹..."
              allowClear
              @keyup.enter="searchDirectories"
            >
              <template #prefix>
                <SearchOutlined style="color: #999;" />
              </template>
            </a-input>
          </div>
          
          <!-- 文件夹列表 -->
          <div class="directory-list">
            <a-list :data-source="filteredDirectories" size="small">
              <template #renderItem="{ item }">
                <a-list-item
                  class="directory-item"
                  :class="{ 'selected': selectedPath === item.path }"
                  @click="selectDirectory(item)"
                  @dblclick="enterDirectory(item)"
                >
                  <a-list-item-meta>
                    <template #avatar>
                      <FolderOutlined style="color: #faad14" />
                    </template>
                    <template #title>
                      <!-- 高亮匹配的文本 -->
                      <span v-html="highlightSearchKeyword(item.name)"></span>
                    </template>
                    <template #description>
                      <span style="font-size: 12px; color: #999;">
                        {{ formatDate(item.modTime) }}
                      </span>
                    </template>
                  </a-list-item-meta>
                </a-list-item>
              </template>
              <!-- 无搜索结果时的提示 -->
              <template #locale>
                <a-empty 
                  image="simple" 
                  :description="searchKeyword ? '没有找到匹配的文件夹' : '当前目录没有子文件夹'"
                />
              </template>
            </a-list>
          </div>
          
          <!-- 新文件名输入 -->
          <div class="file-input" v-if="showFileInput">
            <a-divider />
            <a-form layout="vertical">
              <a-form-item label="文件名">
                <a-input
                  v-model:value="fileName"
                  placeholder="输入文件名，例如：default.conf"
                  @keyup.enter="handleOk"
                />
              </a-form-item>
            </a-form>
          </div>
          
          <!-- 选中路径显示 -->
          <div class="selected-path" v-if="selectedPath">
            <a-divider />
            <div style="padding: 8px; background: #f5f5f5; border-radius: 4px;">
              <strong>将要创建：</strong>
              <br />
              <code>{{ getFullFilePath() }}</code>
            </div>
          </div>
        </div>
      </a-spin>
      
      <template #footer>
        <a-space>
          <a-button @click="handleCancel">取消</a-button>
          <a-button 
            type="primary" 
            @click="handleOk"
            :disabled="!selectedPath || !fileName"
          >
            确定
          </a-button>
        </a-space>
      </template>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { message } from 'ant-design-vue';
import { FolderOutlined, SearchOutlined } from '@ant-design/icons-vue';
import request from '../utils/request';

// Props
interface Props {
  serverId: number;
  modelValue: boolean;
  showFileInput?: boolean; // 是否显示文件名输入框
  defaultFileName?: string; // 默认文件名
}

const props = withDefaults(defineProps<Props>(), {
  showFileInput: true,
  defaultFileName: ''
});

// Emits
const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  'select': [data: { path: string; fileName: string; fullPath: string }];
}>();

// 响应式数据
const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value)
});

const loading = ref(false);
const currentPath = ref('/');
const directories = ref<any[]>([]);
const selectedPath = ref('');
const fileName = ref('');
const searchKeyword = ref('');

// 计算属性
const pathParts = computed(() => {
  if (!currentPath.value) return [];
  return currentPath.value.split('/').filter(Boolean);
});

// 过滤后的目录列表
const filteredDirectories = computed(() => {
  if (!searchKeyword.value.trim()) {
    return directories.value;
  }
  
  const keyword = searchKeyword.value.toLowerCase();
  return directories.value.filter(item => 
    item.name.toLowerCase().includes(keyword)
  );
});

// 监听文件名变化
watch(() => props.defaultFileName, (newVal) => {
  if (newVal) {
    fileName.value = newVal;
  }
}, { immediate: true });

// 监听对话框打开
watch(visible, (newVal) => {
  if (newVal) {
    loadDirectories('/');
    fileName.value = props.defaultFileName;
    searchKeyword.value = ''; // 清空搜索关键词
  }
});

// 获取指定索引的路径
const getPathFromIndex = (index: number): string => {
  if (!pathParts.value || pathParts.value.length === 0) return '/';
  const parts = pathParts.value.slice(0, index + 1);
  return '/' + parts.join('/');
};

// 获取完整文件路径
const getFullFilePath = (): string => {
  if (!selectedPath.value || !fileName.value) return '';
  const path = selectedPath.value.endsWith('/') ? selectedPath.value : selectedPath.value + '/';
  return path + fileName.value;
};

// 格式化日期
const formatDate = (dateStr: string): string => {
  if (!dateStr) return '未知';
  try {
    const date = new Date(dateStr);
    return date.toLocaleString();
  } catch (error) {
    return '未知';
  }
};

// 加载目录列表
const loadDirectories = async (path: string) => {
  loading.value = true;
  
  // 先更新当前路径，确保path计算正确
  const targetPath = path || '/';
  currentPath.value = targetPath;
  
  try {
    const response = await request.get<any[]>(`/servers/${props.serverId}/files/children`, {
      params: { path: targetPath }
    });
    
    console.log('目录加载请求:', targetPath);
    console.log('目录加载响应:', response);
    
    // 确保响应数据是数组
    let data: any[] = response;
    if (!Array.isArray(data)) {
      console.error('响应数据不是数组:', data);
      data = [];
    }
    
    // 过滤只显示目录并添加path字段
    directories.value = data
      .filter((item: any) => item && item.is_dir === true)
      .map((item: any) => {
        // 计算目录的完整路径
        const dirPath = targetPath === '/' 
          ? `/${item.name}` 
          : `${targetPath}/${item.name}`;
        
        return {
          ...item,
          path: dirPath,
          modTime: item.mod_time // 确保字段名一致
        };
      })
      .sort((a: any, b: any) => a.name.localeCompare(b.name));
    
    console.log('加载目录完成:', directories.value.length, '个目录');
    console.log('目录列表:', directories.value.map(d => ({ name: d.name, path: d.path })));
  } catch (error) {
    console.error('加载目录失败:', error);
    message.error('加载目录失败');
    directories.value = [];
  } finally {
    loading.value = false;
  }
};

// 搜索目录（当按下回车键时触发）
const searchDirectories = () => {
  // 搜索功能主要由 filteredDirectories 计算属性实现
  // 这里可以添加额外的搜索逻辑，比如记录搜索历史等
  console.log('搜索关键词:', searchKeyword.value);
};

// 高亮搜索关键词
const highlightSearchKeyword = (text: string): string => {
  if (!searchKeyword.value.trim()) {
    return text;
  }
  
  const keyword = searchKeyword.value.trim();
  const regex = new RegExp(`(${keyword})`, 'gi');
  return text.replace(regex, '<mark style="background-color: #fffacd; padding: 0 2px;">$1</mark>');
};

// 导航到指定路径
const navigateToPath = (path: string) => {
  loadDirectories(path);
  selectedPath.value = path;
  searchKeyword.value = ''; // 清空搜索关键词
};

// 选择目录
const selectDirectory = (item: any) => {
  selectedPath.value = item.path;
};

// 进入目录
const enterDirectory = (item: any) => {
  console.log('进入目录:', item.name, '路径:', item.path);
  loadDirectories(item.path);
  selectedPath.value = item.path;
  searchKeyword.value = ''; // 清空搜索关键词
};

// 确定选择
const handleOk = () => {
  if (!selectedPath.value) {
    message.error('请选择一个目录');
    return;
  }
  
  if (props.showFileInput && !fileName.value) {
    message.error('请输入文件名');
    return;
  }
  
  const result = {
    path: selectedPath.value,
    fileName: fileName.value,
    fullPath: getFullFilePath()
  };
  
  emit('select', result);
  handleCancel();
};

// 取消选择
const handleCancel = () => {
  visible.value = false;
  selectedPath.value = '';
  fileName.value = props.defaultFileName;
};

// 暴露方法给父组件
defineExpose({
  loadDirectories,
  navigateToPath
});
</script>

<style scoped>
.directory-selector {
  /* 组件容器样式 */
}

.directory-browser {
  min-height: 400px;
}

.current-path {
  padding: 12px;
  background: #fafafa;
  border-radius: 4px;
  margin-bottom: 16px;
  border: 1px solid #e8e8e8;
}

.search-box {
  margin-bottom: 16px;
}

.search-box .ant-input {
  border-radius: var(--radius-xs);
}

.search-box :deep(.ant-input-affix-wrapper .ant-input) {
  border: none !important;
  box-shadow: none !important;
  background: transparent !important;
  border-radius: 0 !important;
}

.file-input :deep(.ant-input) {
  border: none !important;
  box-shadow: none !important;
  background: transparent !important;
  border-radius: 0 !important;
}

.directory-list {
  max-height: 300px;
  overflow-y: auto;
  border: 1px solid #e8e8e8;
  border-radius: 4px;
}

.directory-item {
  cursor: pointer;
  padding: 8px 16px;
  transition: background-color 0.3s;
}

.directory-item:hover {
  background-color: #f0f8ff;
}

.directory-item.selected {
  background-color: #e6f7ff;
  border: 1px solid var(--primary-color);
}

.file-input {
  margin-top: 16px;
}

.selected-path {
  margin-top: 16px;
}

.selected-path code {
  color: var(--primary-color);
  font-weight: bold;
}
</style> 
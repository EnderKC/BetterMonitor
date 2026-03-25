import re

with open("/root/project/better_monitor_github/frontend/src/views/server/ServerFile.vue", "r") as f:
    content = f.read()

# 1. Update imports and variables
# Add containerId and containerName
# Find: const serverId = ref<number>(Number(route.params.id));
container_vars = """const serverId = ref<number>(Number(route.params.id));
const containerId = ref<string>(route.params.containerId as string);
const containerName = (route.query.name as string) || containerId.value;
const basePrefix = computed(() => `/servers/${serverId.value}/docker/containers/${containerId.value}`);
const buildUrl = (suffix: string) => `${basePrefix.value}${suffix}`;
"""
content = content.replace("const serverId = ref<number>(Number(route.params.id));", container_vars)

# 2. Update API endpoints
content = content.replace("`/servers/${serverId.value}/files`", "buildUrl('/files')")
content = content.replace("`/servers/${serverId.value}/files/children`", "buildUrl('/files/children')")
content = content.replace("`/servers/${serverId.value}/files/content`", "buildUrl('/files/content')")
content = content.replace("`/servers/${serverId.value}/files/delete`", "buildUrl('/files/delete')")
content = content.replace("`/servers/${serverId.value}/files/create`", "buildUrl('/files/create')")
content = content.replace("`/servers/${serverId.value}/files/mkdir`", "buildUrl('/files/mkdir')")

# 3. Update download URL
download_url_orig = "const downloadUrl = `${window.location.origin}/api/servers/${serverId.value}/files/download?path=${encodeURIComponent(filePath)}&token=${token}`;"
download_url_new = "const downloadUrl = `${window.location.origin}/api/servers/${serverId.value}/docker/containers/${containerId.value}/files/download?path=${encodeURIComponent(filePath)}&token=${token}`;"
content = content.replace(download_url_orig, download_url_new)

# 4. Update upload logic
upload_orig = """await uploadFile({
      serverId: serverId.value,
      targetPath: currentPath.value,
      file: fileToUpload.value,
    });"""
upload_new = """await uploadFile({
      serverId: serverId.value,
      containerId: containerId.value,
      targetPath: currentPath.value,
      file: fileToUpload.value,
    });"""
content = content.replace(upload_orig, upload_new)

# 5. Hide terminal button
# We will just remove the terminal button from toolbar
terminal_btn = """          <a-button class="action-btn" @click="openTerminal" title="在当前目录打开终端">
            <CodeOutlined />
          </a-button>"""
content = content.replace(terminal_btn, "")

# 6. Update goBack logic
# ServerFile has a goBack that goes to /admin/servers/X
# We want to go back to /admin/servers/X/docker
go_back_orig = """const goBack = () => {
  const from = route.query.from;
  if (from === 'nginx') {
    router.push(`/admin/servers/${serverId.value}/nginx`);
    return;
  }
  if (from === 'docker') {
    router.push(`/admin/servers/${serverId.value}/docker`);
    return;
  }
  router.push(`/admin/servers/${serverId.value}`);
};"""
go_back_new = """const goBack = () => {
  router.push(`/admin/servers/${serverId.value}/docker`);
};"""
content = content.replace(go_back_orig, go_back_new)

# 7. Update window title and window-controls
window_title_orig = """<span class="window-title">{{ serverInfo.name || '文件管理' }}</span>"""
window_title_new = """<span class="window-title">{{ containerName || '容器文件管理' }}</span>"""
content = content.replace(window_title_orig, window_title_new)

with open("/root/project/better_monitor_github/frontend/src/views/server/ServerDockerFile.vue", "w") as f:
    f.write(content)

print("Transform complete.")

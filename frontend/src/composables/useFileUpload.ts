import { reactive, onBeforeUnmount } from 'vue'
import axios from 'axios'
import request from '../utils/request'
import { getToken } from '../utils/auth'
import type { AxiosProgressEvent } from 'axios'

/** 分片上传阈值：超过此大小自动启用分片上传 */
const CHUNKED_THRESHOLD = 5 * 1024 * 1024 // 5MB
/** 每个分片大小 */
const CHUNK_SIZE = 2 * 1024 * 1024 // 2MB

/** 不可压缩的文件扩展名（已经是压缩格式） */
const INCOMPRESSIBLE_EXTENSIONS = new Set([
  '.zip', '.gz', '.tar.gz', '.tgz', '.bz2', '.xz', '.7z', '.rar',
  '.jpg', '.jpeg', '.png', '.gif', '.webp', '.avif', '.ico', '.bmp',
  '.mp4', '.avi', '.mkv', '.mov', '.webm', '.flv', '.wmv',
  '.mp3', '.aac', '.ogg', '.flac', '.wma', '.m4a',
  '.pdf', '.woff', '.woff2', '.ttf', '.otf', '.eot',
  '.exe', '.dll', '.so', '.dylib',
  '.zst', '.lz4', '.br',
])

/**
 * 判断文件是否适合压缩（基于扩展名）
 * 已经是压缩格式的文件压缩后反而可能增大，应跳过
 */
const isCompressible = (filename: string): boolean => {
  const lower = filename.toLowerCase()
  return !Array.from(INCOMPRESSIBLE_EXTENSIONS).some(ext => lower.endsWith(ext))
}

/**
 * 使用 CompressionStream('gzip') 压缩 ArrayBuffer
 * 需要浏览器支持 CompressionStream API
 */
const compressChunk = async (input: ArrayBuffer): Promise<ArrayBuffer> => {
  const compressed = new Blob([input])
    .stream()
    .pipeThrough(new CompressionStream('gzip'))
  return new Response(compressed).arrayBuffer()
}

/** 上传配置选项 */
export interface UploadOptions {
  /** 服务器 ID */
  serverId: number | string
  /** 容器 ID（有值则走容器上传接口） */
  containerId?: string
  /** 目标目录路径 */
  targetPath: string
  /** 要上传的文件 */
  file: File
  /** 进度回调 */
  onProgress?: (percent: number) => void
}

/** 上传状态 */
export interface UploadState {
  /** 是否正在上传 */
  uploading: boolean
  /** 上传进度百分比 (0-100) */
  progress: number
  /** 错误信息 */
  error: string
  /** 上传速度（字节/秒） */
  speed: number
  /** 已上传字节数 */
  uploadedSize: number
  /** 文件总字节数 */
  totalSize: number
  /** 当前文件名 */
  fileName: string
  /** 当前分片索引（分片模式下有效，从 1 开始） */
  currentChunk: number
  /** 总分片数（分片模式下 > 0） */
  totalChunks: number
}

/**
 * 统一文件上传 composable
 *
 * 支持主机文件上传和容器文件上传，提供进度追踪、取消上传能力。
 * 文件 > 5MB 时自动切换到分片上传模式。
 */
export function useFileUpload() {
  const state = reactive<UploadState>({
    uploading: false,
    progress: 0,
    error: '',
    speed: 0,
    uploadedSize: 0,
    totalSize: 0,
    fileName: '',
    currentChunk: 0,
    totalChunks: 0,
  })

  let abortController: AbortController | null = null
  let uploadStartTime = 0
  /** 分片上传模式下的 upload_id，用于取消 */
  let activeUploadId: string | null = null
  /** 分片上传模式下的 serverId，用于取消 */
  let activeServerId: number | string | null = null

  /** 重置状态 */
  const reset = () => {
    state.uploading = false
    state.progress = 0
    state.error = ''
    state.speed = 0
    state.uploadedSize = 0
    state.totalSize = 0
    state.fileName = ''
    state.currentChunk = 0
    state.totalChunks = 0
    activeUploadId = null
    activeServerId = null
  }

  /** 根据上传目标构建 API 端点 */
  const buildEndpoint = (serverId: number | string, containerId?: string): string => {
    if (containerId) {
      return `/servers/${serverId}/docker/containers/${containerId}/files/upload`
    }
    return `/servers/${serverId}/files/upload`
  }

  /** 更新上传速度 */
  const updateSpeed = (uploadedBytes: number) => {
    const elapsedMs = Date.now() - uploadStartTime
    if (elapsedMs > 0) {
      state.speed = Math.round((uploadedBytes / elapsedMs) * 1000)
    }
  }

  /**
   * 计算字节数组的 SHA-256 哈希（十六进制字符串）
   */
  const computeSHA256 = async (data: ArrayBuffer): Promise<string> => {
    const hashBuffer = await crypto.subtle.digest('SHA-256', data)
    const hashArray = Array.from(new Uint8Array(hashBuffer))
    return hashArray.map(b => b.toString(16).padStart(2, '0')).join('')
  }

  /**
   * 普通上传（小文件，multipart/form-data）
   */
  const uploadSimple = async (options: UploadOptions): Promise<void> => {
    const formData = new FormData()
    formData.append('file', options.file)
    formData.append('path', options.targetPath)

    const endpoint = buildEndpoint(options.serverId, options.containerId)

    await request.post(endpoint, formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
      signal: abortController!.signal,
      timeout: 0,
      onUploadProgress: (event: AxiosProgressEvent) => {
        const loaded = event.loaded ?? 0
        const total = event.total ?? options.file.size

        state.uploadedSize = loaded
        state.totalSize = total
        state.progress = total > 0 ? Math.min(99, Math.round((loaded / total) * 100)) : 0

        updateSpeed(loaded)
        options.onProgress?.(state.progress)
      },
    })

    state.progress = 100
    state.uploadedSize = state.totalSize
  }

  /**
   * 生成文件指纹，用于断点续传时识别同一文件
   */
  const computeFileFingerprint = (file: File, serverId: number | string, targetPath: string): string => {
    return `chunked_upload_${serverId}_${targetPath}_${file.name}_${file.size}_${file.lastModified}`
  }

  /**
   * 尝试恢复已有的分片上传会话（断点续传）
   * 返回 { uploadId, receivedChunks } 或 null
   */
  const tryResumeUpload = async (
    fingerprint: string,
    serverId: number | string,
    signal: AbortSignal,
  ): Promise<{ uploadId: string; receivedChunks: number[] } | null> => {
    try {
      const savedId = localStorage.getItem(fingerprint)
      if (!savedId) return null

      // 使用原生 axios 绕过全局响应拦截器：会话不存在时返回 404 是预期行为，不应弹出错误提示
      const token = getToken()
      const resp = await axios.get(
        `/servers/${serverId}/files/upload/chunked/${savedId}/status`,
        {
          baseURL: request.defaults.baseURL,
          headers: token ? { Authorization: `Bearer ${token}` } : {},
          signal,
          timeout: 10000,
        },
      )
      const statusResp = resp.data

      if (statusResp.status === 'ready' || statusResp.status === 'uploading') {
        const received: number[] = statusResp.received_chunks ?? []
        return { uploadId: savedId, receivedChunks: received }
      }

      // 会话已完成/失败/取消，清除缓存
      localStorage.removeItem(fingerprint)
      return null
    } catch {
      // 会话不存在或查询失败，清除缓存
      localStorage.removeItem(fingerprint)
      return null
    }
  }

  /**
   * 分片上传（大文件），支持断点续传
   */
  const uploadChunked = async (options: UploadOptions): Promise<void> => {
    const file = options.file
    const totalChunks = Math.ceil(file.size / CHUNK_SIZE)

    state.totalChunks = totalChunks
    state.currentChunk = 0

    const fingerprint = computeFileFingerprint(file, options.serverId, options.targetPath)
    let uploadId: string
    let receivedSet = new Set<number>()

    // 尝试断点续传
    const resumed = await tryResumeUpload(fingerprint, options.serverId, abortController!.signal)
    if (resumed) {
      uploadId = resumed.uploadId
      receivedSet = new Set(resumed.receivedChunks)
      state.currentChunk = receivedSet.size
      state.uploadedSize = Math.min(receivedSet.size * CHUNK_SIZE, file.size)
      state.progress = Math.min(99, Math.round((receivedSet.size / totalChunks) * 100))
    } else {
      // 1. 初始化分片上传会话
      const initResp = await request.post(
        `/servers/${options.serverId}/files/upload/chunked/init`,
        {
          path: options.targetPath,
          filename: file.name,
          total_size: file.size,
          chunk_size: CHUNK_SIZE,
          total_chunks: totalChunks,
          container_id: options.containerId || '',
        },
        { signal: abortController!.signal, timeout: 30000 },
      )

      uploadId = initResp.upload_id as string
      if (!uploadId) {
        throw new Error('初始化分片上传失败：未返回 upload_id')
      }

      // 保存到 localStorage 用于断点续传
      localStorage.setItem(fingerprint, uploadId)
    }

    activeUploadId = uploadId
    activeServerId = options.serverId

    // 2. 判断是否启用压缩（文件可压缩 且 浏览器支持 CompressionStream）
    const useCompression = isCompressible(file.name) && typeof CompressionStream !== 'undefined'

    // 3. 逐片上传（跳过已接收的分片）
    for (let i = 0; i < totalChunks; i++) {
      // 跳过已接收的分片
      if (receivedSet.has(i)) {
        continue
      }

      // 检查是否已取消
      if (abortController!.signal.aborted) {
        throw new DOMException('上传已取消', 'AbortError')
      }

      const start = i * CHUNK_SIZE
      const end = Math.min(start + CHUNK_SIZE, file.size)
      const chunkBlob = file.slice(start, end)
      const chunkBuffer = await chunkBlob.arrayBuffer()

      // 压缩分片（如果启用），压缩失败时自动降级为原始数据
      let uploadBuffer: ArrayBuffer = chunkBuffer
      let chunkCompressed = false
      if (useCompression) {
        try {
          uploadBuffer = await compressChunk(chunkBuffer)
          chunkCompressed = true
        } catch {
          // CompressionStream 运行时异常，降级为不压缩
          uploadBuffer = chunkBuffer
        }
      }

      // 计算传输数据的 SHA-256 哈希（压缩后的数据，保证传输完整性）
      const chunkHash = await computeSHA256(uploadBuffer)

      // 构造请求头
      const headers: Record<string, string> = {
        'Content-Type': 'application/octet-stream',
        'X-Chunk-Hash': chunkHash,
      }
      if (chunkCompressed) {
        headers['X-Chunk-Compressed'] = 'gzip'
      }

      // 发送分片（二进制 body）
      await request.put(
        `/servers/${options.serverId}/files/upload/chunked/${uploadId}/chunk/${i}`,
        uploadBuffer,
        {
          headers,
          signal: abortController!.signal,
          timeout: 0,
        },
      )

      // 更新进度
      receivedSet.add(i)
      state.currentChunk = receivedSet.size
      state.uploadedSize = Math.min(receivedSet.size * CHUNK_SIZE, file.size)
      state.progress = Math.min(99, Math.round((receivedSet.size / totalChunks) * 100))

      updateSpeed(state.uploadedSize)
      options.onProgress?.(state.progress)
    }

    // 3. 请求合并
    await request.post(
      `/servers/${options.serverId}/files/upload/chunked/${uploadId}/complete`,
      {},
      { signal: abortController!.signal, timeout: 120000 },
    )

    state.progress = 100
    state.uploadedSize = state.totalSize
    activeUploadId = null
    activeServerId = null

    // 清除 localStorage 中的续传记录
    localStorage.removeItem(fingerprint)
  }

  /**
   * 上传文件（自动选择普通上传或分片上传）
   *
   * @throws 上传失败或被取消时抛出错误
   */
  const uploadFile = async (options: UploadOptions): Promise<void> => {
    if (state.uploading) {
      throw new Error('上传正在进行中')
    }
    if (!options.file) {
      throw new Error('请选择要上传的文件')
    }

    reset()
    state.uploading = true
    state.totalSize = options.file.size
    state.fileName = options.file.name
    uploadStartTime = Date.now()
    abortController = new AbortController()

    try {
      if (options.file.size > CHUNKED_THRESHOLD) {
        try {
          await uploadChunked(options)
        } catch (chunkedErr: any) {
          // 分片初始化失败（如 Agent 不支持）时，回退到普通上传
          const isInitFailure = !activeUploadId
          const isCancelled = chunkedErr?.name === 'CanceledError' || chunkedErr?.name === 'AbortError'
          if (isInitFailure && !isCancelled) {
            // 重置分片状态，回退普通上传
            state.currentChunk = 0
            state.totalChunks = 0
            state.progress = 0
            state.uploadedSize = 0
            uploadStartTime = Date.now()
            await uploadSimple(options)
          } else {
            throw chunkedErr
          }
        }
      } else {
        await uploadSimple(options)
      }
    } catch (err: any) {
      if (err?.name === 'CanceledError' || err?.code === 'ERR_CANCELED' || err?.name === 'AbortError') {
        state.error = '上传已取消'
      } else {
        state.error = err?.response?.data?.error || err?.error || err?.message || '上传失败'
      }
      throw err
    } finally {
      state.uploading = false
      abortController = null
    }
  }

  /** 取消当前上传 */
  const cancelUpload = () => {
    abortController?.abort()

    // 分片模式下，通知后端取消并清理临时文件
    if (activeUploadId && activeServerId) {
      const uploadId = activeUploadId
      const serverId = activeServerId
      activeUploadId = null
      activeServerId = null

      // 使用原生 axios 绕过全局响应拦截器，避免取消时弹出错误提示
      const token = getToken()
      axios.delete(`/servers/${serverId}/files/upload/chunked/${uploadId}`, {
        baseURL: request.defaults.baseURL,
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      }).catch((err: any) => {
        console.warn('取消上传清理请求失败（可忽略）:', err?.message || err)
      })
    }
  }

  // 组件卸载时自动取消未完成的上传
  onBeforeUnmount(() => {
    cancelUpload()
  })

  return {
    state,
    uploadFile,
    cancelUpload,
    reset,
  }
}

/**
 * 格式化文件大小为人类可读字符串
 */
export function formatFileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'

  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unitIndex = 0

  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex++
  }

  const decimals = value >= 100 ? 0 : value >= 10 ? 1 : 2
  return `${value.toFixed(decimals)} ${units[unitIndex]}`
}

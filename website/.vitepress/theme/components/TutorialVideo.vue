<script setup lang="ts">
// 教程页的演示视频：视频与封面在 public/videos/tutorials/<name>.<语言>.mp4 / .jpg（不进 git，随 deploy:site 上传）。
// 只在点击播放后才下载视频，封面图先占位。
import { withBase } from 'vitepress'

const props = defineProps<{
  name: string
  // 视频的语言后缀，目前只有简体中文录制
  lang?: string
  // 视频下方的说明文字
  caption?: string
}>()

const base = `/videos/tutorials/${props.name}.${props.lang ?? 'zh-CN'}`
</script>

<template>
  <figure class="tut-video-box">
    <video controls preload="none" playsinline :poster="withBase(`${base}.jpg`)" :src="withBase(`${base}.mp4`)" />
    <figcaption v-if="caption">{{ caption }}</figcaption>
  </figure>
</template>

<style scoped>
.tut-video-box {
  margin: 20px 0 24px;
}
.tut-video-box video {
  display: block;
  width: 100%;
  aspect-ratio: 1440 / 900;
  border: 1px solid var(--vp-c-divider);
  border-radius: 10px;
  background: #0b3f35;
}
.tut-video-box figcaption {
  margin-top: 8px;
  font-size: 13px;
  color: var(--vp-c-text-2);
  text-align: center;
}
</style>

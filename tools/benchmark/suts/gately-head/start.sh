# 新版 Gately（本地构建，镜像 gately:bench-head = 官方 v0.6.8.1 镜像替换程序文件），配置与 suts/gately 相同
GATELY_IMAGE=${GATELY_HEAD_IMAGE:-gately:bench-head} bash $(dirname "$0")/../gately/start.sh

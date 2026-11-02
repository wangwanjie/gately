// Docker 管理界面使用的接口类型，字段与 docs/DOCKER_API.md 一一对应。

export interface UpdateState {
  checked: string
  hasUpdate: boolean
  error?: string
}

export interface ContainerView {
  id: string
  name: string
  image: string
  imageId?: string
  state: string
  status: string
  ports: string[] | null
  project?: string
  busy: boolean
  self?: boolean
  networks?: string[]
  mounts?: number
  started?: string
  update?: UpdateState
}

export interface PortSpec {
  hostIp: string
  host: number | null
  container: number | null
  proto: string
}

export interface VolumeSpec {
  type: 'bind' | 'volume'
  source: string
  target: string
  readOnly: boolean
}

export interface DeviceSpec {
  host: string
  container: string
  perm: string
}

export interface ContainerSpec {
  name: string
  image: string
  pull: boolean
  autoStart: boolean
  command: string
  entrypoint: string
  env: string[]
  ports: PortSpec[]
  volumes: VolumeSpec[]
  network: string
  networks: string[]
  ip: string
  aliases: string[]
  restart: string
  restartMaxRetries: number
  privileged: boolean
  capAdd: string[]
  capDrop: string[]
  devices: DeviceSpec[]
  user: string
  workdir: string
  hostname: string
  dns: string[]
  labels: Record<string, string>
  memoryMB: number
  cpus: number
  tty: boolean
  stdinOpen: boolean
}

export interface ContainerDetail extends ContainerSpec {
  id: string
  state: string
  status: string
  created: string
  started: string
  imageId: string
  path: string
  args: string[]
  mounts: { type: string; name: string; source: string; destination: string; rw: boolean }[]
  networkDetails: { name: string; ip: string; gateway: string; mac: string; aliases: string[] }[]
  exitCode: number
  restartCount: number
  healthStatus: string
  composeProject: string
  composeService: string
}

export interface ContainerStats {
  cpuPercent: number
  memUsage: number
  memLimit: number
  memPercent: number
  netRx: number
  netTx: number
  blockRead: number
  blockWrite: number
  pids: number
}

export interface FileEntry {
  name: string
  path: string
  dir: boolean
  size: number
  mode: string
  modified: string
  link?: string
}

export interface DockerInfo {
  version: string
  apiVersion: string
  os: string
  arch: string
  kernel: string
  name: string
  cpus: number
  memTotal: number
  storageDriver: string
  rootDir: string
  containers: { total: number; running: number; paused: number; stopped: number }
  images: number
  composeCli: boolean
  composeCliVersion: string
  daemonMirrors: string[]
}

export interface DiskItem {
  count: number
  size: number
  reclaimable: number
}

export interface DiskUsage {
  images: DiskItem
  containers: DiskItem
  volumes: DiskItem
  buildCache: DiskItem
}

export interface DockerSettings {
  host: string
  mirrors: string[]
  registries: { server: string; username: string; password: string }[]
  autoCheckHours: number
}

export interface DockerTask {
  id: string
  kind: string
  title: string
  status: 'running' | 'done' | 'error' | 'cancelled'
  error?: string
  // cancelling 表示已请求取消、任务正在收尾
  cancelling?: boolean
  lines?: string[]
  started: string
  finished?: string
}

export interface ComposeService {
  name: string
  state: string
  containerId: string
  image: string
  ports: string[]
}

export interface ComposeProject {
  name: string
  workingDir: string
  configFiles: string[]
  status: 'running' | 'partial' | 'stopped'
  total: number
  running: number
  services: ComposeService[]
  managed: boolean
  self?: boolean
}

export interface ImageView {
  id: string
  tags: string[]
  digests: string[]
  size: number
  created: number | string
  containers: number
  dangling: boolean
}

export interface ImageDetail {
  id: string
  tags: string[]
  size: number
  created: number | string
  os: string
  arch: string
  author: string
  cmd: string[]
  entrypoint: string[]
  env: string[]
  exposedPorts: string[]
  workdir: string
  user: string
  labels: Record<string, string>
  layers: { createdBy: string; size: number; created: number | string }[]
}

export interface ImageSearchItem {
  name: string
  description: string
  stars: number
  official: boolean
}

export interface NetworkView {
  id: string
  name: string
  driver: string
  scope: string
  subnet: string
  gateway: string
  internal: boolean
  ipv6: boolean
  builtin: boolean
  containers: { id: string; name: string; ip: string }[] | null
}

export interface VolumeView {
  name: string
  driver: string
  mountpoint: string
  created: string
  size: number
  refCount: number
  usedBy: { id: string; name: string }[] | null
  labels: Record<string, string> | null
}

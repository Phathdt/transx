import type { Config } from '@react-router/dev/config'

export default {
  // Framework mode with SSR. Auth loaders stay auth-gate only.
  ssr: true,
  appDirectory: 'app',
} satisfies Config

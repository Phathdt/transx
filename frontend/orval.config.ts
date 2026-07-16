import { defineConfig } from 'orval'

const openapi = '../backend/openapi.yaml'

export default defineConfig({
  // Browser domain APIs (wallet/transfer/inbox) + React Query hooks.
  api: {
    input: { target: openapi },
    output: {
      mode: 'tags-split',
      target: 'src/lib/api/generated',
      schemas: 'src/lib/api/generated/models',
      client: 'react-query',
      httpClient: 'axios',
      override: {
        mutator: {
          path: 'src/lib/api/http-mutator.ts',
          name: 'apiClient',
        },
      },
    },
  },

  // Server-only fetch client for RR loaders/actions (BFF → Go).
  // auth / wallet lists / inbox unread for SSR shell badge.
  apiServer: {
    input: {
      target: openapi,
      filters: {
        mode: 'include',
        tags: ['auth', 'wallet', 'inbox'],
      },
    },
    output: {
      mode: 'tags-split',
      target: 'app/lib/api/generated',
      schemas: 'app/lib/api/generated/models',
      client: 'fetch',
      clean: true,
      override: {
        mutator: {
          path: 'app/lib/api/server-http-mutator.ts',
          name: 'serverApiClient',
        },
        fetch: {
          includeHttpResponseReturnType: false,
        },
      },
    },
  },

  apiZod: {
    input: { target: openapi },
    output: {
      mode: 'tags-split',
      target: 'src/lib/api/generated',
      client: 'zod',
      fileExtension: '.zod.ts',
    },
  },
})

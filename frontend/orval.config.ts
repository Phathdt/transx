import { defineConfig } from 'orval'

export default defineConfig({
  api: {
    input: { target: '../backend/openapi.yaml' },
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
  apiZod: {
    input: { target: '../backend/openapi.yaml' },
    output: {
      mode: 'tags-split',
      target: 'src/lib/api/generated',
      client: 'zod',
      fileExtension: '.zod.ts',
    },
  },
})
